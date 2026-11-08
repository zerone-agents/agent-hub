package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"control-panel/internal/domain/provider"
)

func instanceCandidate(p provider.Provider) (string, provider.InstanceCandidate, error) {
	input := provider.InstanceCandidate{APIKey: p.LockedAPIKey(), BaseURL: p.BaseURL(), ModelIDs: []string{}, CustomModels: []provider.ModelRegistration{}}
	factory := map[string]string{"openai": "OpenAI-API-Compatible", "anthropic": "Anthropic", "mineru": "MinerU", "paddleocr": "PaddleOCR"}[p.Protocol()]
	if factory == "" {
		return "", input, provider.NewValidationErrorf("该协议不能同步到MultiRAG")
	}
	if p.Protocol() == "mineru" || p.Protocol() == "paddleocr" {
		if strings.TrimSpace(p.BaseURL()) == "" {
			return "", input, provider.NewValidationErrorf("OCR 服务地址不能为空")
		}
		key := map[string]string{}
		if p.Protocol() == "mineru" {
			key["mineru_apiserver"] = p.BaseURL()
			for field, name := range map[string]string{"backend": "mineru_backend", "output_dir": "mineru_output_dir", "server_url": "mineru_server_url"} {
				if attr, ok := p.Attributes()[field]; ok {
					key[name] = attr.Value
				}
			}
			if attr, ok := p.Attributes()["delete_output"]; ok {
				key["mineru_delete_output"] = "0"
				if attr.Value == "true" {
					key["mineru_delete_output"] = "1"
				}
			}
		} else {
			key["paddleocr_api_url"] = p.BaseURL()
			for field, name := range map[string]string{"algorithm": "paddleocr_algorithm", "access_token": "paddleocr_access_token"} {
				if attr, ok := p.Attributes()[field]; ok {
					key[name] = attr.Value
				}
			}
		}
		raw, _ := json.Marshal(key)
		input.APIKey = string(raw)
		input.BaseURL = ""
	} else if input.APIKey == "" && p.AuthStyle() == "no_auth" {
		input.APIKey = "x"
	}
	if input.APIKey == "" {
		return "", input, provider.NewValidationErrorf("同步模型需要配置服务密钥")
	}
	byName := map[string]int{}
	for _, model := range p.DefaultModels() {
		if model.Status == "0" {
			continue
		}
		name, capability := model.ModelID, provider.MapModelTypeToMultiRAG(model.ModelType)
		isParser := p.Protocol() == "mineru" || p.Protocol() == "paddleocr"
		if (isParser && capability != "ocr") || (!isParser && capability == "ocr") {
			return "", input, provider.NewValidationErrorf("该供应商协议不支持所选模型能力")
		}
		if name == "" || strings.TrimSpace(name) != name || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 128 || strings.Contains(name, "@") || capability == "" || model.ContextWindow < 0 || model.ContextWindow > 2147483647 {
			return "", input, provider.NewValidationErrorf("模型名称、类型或上下文长度不符合MultiRAG实例合同")
		}
		if index, ok := byName[name]; ok {
			existing := &input.CustomModels[index]
			if existing.MaxTokens != model.ContextWindow {
				return "", input, provider.NewValidationErrorf("同名模型的上下文长度必须一致")
			}
			if !slices.Contains(existing.ModelTypes, capability) {
				existing.ModelTypes = append(existing.ModelTypes, capability)
			}
		} else {
			byName[name] = len(input.CustomModels)
			input.CustomModels = append(input.CustomModels, provider.ModelRegistration{ModelName: name, ModelTypes: []string{capability}, MaxTokens: model.ContextWindow})
		}
	}
	if len(input.CustomModels) == 0 || len(input.CustomModels) > 100 {
		return "", input, provider.NewValidationErrorf("请选择1到100个启用模型")
	}
	return factory, input, nil
}

func configHash(input provider.InstanceCandidate) string {
	raw, _ := json.Marshal([]string{input.APIKey, input.BaseURL, input.Region})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func matchingModels(rows []provider.RemoteModel, instanceID string, input provider.InstanceCandidate) ([]provider.SyncCallResult, error) {
	result := make([]provider.SyncCallResult, 0, len(input.CustomModels))
	for _, selected := range input.CustomModels {
		matches := []provider.RemoteModel{}
		for _, row := range rows {
			if row.Name == selected.ModelName {
				matches = append(matches, row)
			}
		}
		if len(matches) != 1 {
			return nil, fmt.Errorf("MultiRAG model readback is incomplete; refresh to confirm")
		}
		row := matches[0]
		if row.InstanceID != instanceID || !provider.ValidModelReference(row.Reference) || row.EffectiveStatus != "active" {
			return nil, fmt.Errorf("MultiRAG model identity or configuration did not match")
		}
		if row.MaxTokens != selected.MaxTokens {
			return nil, provider.NewValidationErrorf("绑定模型的上下文声明与本地不同，请在MultiRAG核对；不能自动替换已有引用")
		}
		for _, capability := range selected.ModelTypes {
			if !slices.Contains(row.ModelTypes, capability) {
				return nil, provider.NewValidationErrorf("绑定实例中的同名模型不支持所选能力；请在MultiRAG管理页核对，不能自动替换已有引用")
			}
		}
		result = append(result, provider.SyncCallResult{ModelName: selected.ModelName, Reference: row.Reference, HTTPStatus: 200, OK: true})
	}
	return result, nil
}

func (s *ProviderService) currentSyncCandidate(tenantID string, id uint64, selected []string, factory string, expected provider.InstanceCandidate) error {
	current, err := s.GetByID(tenantID, id)
	if err != nil {
		return err
	}
	key, err := provider.Decrypt(current.LockedAPIKey(), s.encryptionKey)
	if err != nil {
		return err
	}
	current.Base().SetLockedAPIKey(key)
	attrs, err := s.repo.GetAttributes(tenantID, id)
	if err != nil {
		return err
	}
	current.Base().SetAttributes(attrs)
	if len(selected) > 0 {
		current.Base().SetDefaultModels(filterModels(current.DefaultModels(), selected))
	}
	currentFactory, input, err := instanceCandidate(current)
	if err != nil {
		return err
	}
	a, _ := json.Marshal(expected)
	b, _ := json.Marshal(input)
	if currentFactory != factory || string(a) != string(b) {
		return provider.NewValidationErrorf("供应商配置或所选模型已变化，请刷新后重新同步")
	}
	return nil
}

func (s *ProviderService) syncProviderInstance(ctx context.Context, tenantID string, p provider.Provider, client provider.MultiRAGInstances, verifyOnly bool, selectedIDs []string) (*provider.SyncResult, error) {
	attrs, err := s.repo.GetAttributes(tenantID, p.ID())
	if err != nil {
		return nil, err
	}
	p.Base().SetAttributes(attrs)
	factory, input, err := instanceCandidate(p)
	if err != nil {
		return nil, err
	}
	instances, err := client.ListInstances(ctx, factory)
	if err != nil {
		return nil, err
	}
	result := &provider.SyncResult{FactoryName: factory, Endpoint: "instances", PerCall: []provider.SyncCallResult{}}
	if verifyOnly {
		if err = client.VerifyInstance(ctx, factory, input); err != nil {
			return nil, err
		}
		result.Success = true
		result.CallCount = 1
		// The model service probes the first selected declaration; it does not prove
		// inference for every capability in the candidate batch.
		result.PerCall = append(result.PerCall, provider.SyncCallResult{ModelName: input.CustomModels[0].ModelName, HTTPStatus: 200, OK: true})
		return result, nil
	}
	store, ok := s.repo.(provider.MultiRAGBindingRepository)
	if !ok {
		return nil, fmt.Errorf("MultiRAG binding persistence unavailable")
	}
	target := client.SyncTarget()
	binding, err := store.GetMultiRAGBinding(tenantID, p.ID(), target)
	if err != nil {
		return nil, err
	}
	created := false
	if binding == nil {
		// Never adopt an unrelated migrated default instance by name/model.
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			return nil, err
		}
		binding = &provider.MultiRAGSyncBinding{ProviderID: p.ID(), Target: target, InstanceName: fmt.Sprintf("hub-%d-%s", p.ID(), hex.EncodeToString(nonce[:])), ConfigHash: configHash(input), State: "creating"}
		binding.Lease = hex.EncodeToString(nonce[:])
		created, err = store.ReserveMultiRAGBinding(tenantID, binding)
		if err != nil {
			return nil, err
		}
		if !created {
			binding, err = store.GetMultiRAGBinding(tenantID, p.ID(), target)
			if err != nil || binding == nil {
				return nil, fmt.Errorf("MultiRAG binding reservation is unconfirmed")
			}
		}
	}
	previousHash := binding.ConfigHash
	wasReady := binding.State == "ready"
	unknownWrite, finished := false, false
	confirmedHash := previousHash
	if wasReady {
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			return nil, err
		}
		binding.Lease = hex.EncodeToString(nonce[:])
		binding.ConfigHash = configHash(input)
		acquired, lockErr := store.BeginMultiRAGSync(tenantID, binding)
		if lockErr != nil {
			return nil, lockErr
		}
		if !acquired {
			return nil, provider.NewValidationErrorf("模型同步正在进行或结果未确认，请刷新核对")
		}
		binding.State = "syncing"
		binding.Version++
	} else if binding.State != "creating" {
		return nil, provider.NewValidationErrorf("上次实例更新结果未确认，请在MultiRAG核对后由管理员恢复绑定；不会重放写入")
	}
	defer func() {
		if wasReady && !unknownWrite && !finished {
			// Read/probe failures and confirmed writes may release this exact lease.
			// Keep the last ACKNOWLEDGED credential digest, never a requested but
			// unknown key or a stale source snapshot. Partial model additions remain
			// visible on the next explicit sync without repeating successful writes.
			binding.ConfigHash = confirmedHash
			_ = store.ConfirmMultiRAGBinding(tenantID, binding)
		}
	}()
	// A request may have waited in inventory I/O while a newer provider snapshot
	// was saved and synced. Acquiring a fresh binding version does not authorize
	// that stale source snapshot to rotate the instance back to its old key.
	if err = s.currentSyncCandidate(tenantID, p.ID(), selectedIDs, factory, input); err != nil {
		if created {
			_ = store.ReleaseMultiRAGBinding(tenantID, binding)
		}
		return nil, err
	}
	result.InstanceName = binding.InstanceName
	var remote *provider.RemoteInstance
	for _, row := range instances {
		if row.Name == binding.InstanceName {
			if remote != nil {
				return nil, fmt.Errorf("ambiguous MultiRAG instance identity")
			}
			copy := row
			remote = &copy
		}
	}
	if created {
		remote, err = client.CreateInstance(ctx, factory, provider.InstanceInput{InstanceCandidate: input, InstanceName: binding.InstanceName})
		if err != nil {
			var rejected interface{ Rejected() bool }
			if errors.As(err, &rejected) && rejected.Rejected() {
				_ = store.ReleaseMultiRAGBinding(tenantID, binding)
			}
			// The reserved identity survives ambiguous failures. A later call only
			// reads that identity; absence never authorizes another creation POST.
			return nil, err
		}
	} else if remote == nil {
		return nil, provider.NewValidationErrorf("上游实例结果未确认或已删除，请先在MultiRAG核对；不会重复创建")
	}
	if remote == nil || remote.Status != "active" || (binding.InstanceID != "" && binding.InstanceID != remote.ID) {
		return nil, provider.NewValidationErrorf("绑定实例已禁用或身份变化，请在MultiRAG核对")
	}
	if binding.State == "creating" && binding.ConfigHash != configHash(input) {
		return nil, provider.NewValidationErrorf("上次创建结果未确认，请先恢复原配置并核对")
	}
	if remote.BaseURL != input.BaseURL || remote.Region != input.Region || remote.HasAPIKey != (input.APIKey != "") || previousHash != configHash(input) {
		if !wasReady {
			return nil, fmt.Errorf("unconfirmed MultiRAG instance configuration")
		}
		if err = client.VerifyInstance(ctx, factory, input); err != nil {
			return nil, err
		}
		if err = s.currentSyncCandidate(tenantID, p.ID(), selectedIDs, factory, input); err != nil {
			return nil, err
		}
		unknownWrite = true
		remote, err = client.UpdateInstance(ctx, factory, binding.InstanceName, input)
		if err != nil {
			var rejected interface{ Rejected() bool }
			if errors.As(err, &rejected) && rejected.Rejected() {
				unknownWrite = false
			}
			return nil, err
		}
		if remote.ID != binding.InstanceID {
			return nil, fmt.Errorf("MultiRAG instance identity changed")
		}
		unknownWrite = false
		confirmedHash = configHash(input)
	}
	rows, err := client.ListInstanceModels(ctx, factory, binding.InstanceName)
	if err != nil {
		return nil, err
	}
	if wasReady {
		// Later additions use the bound instance's singular model API. Existing
		// models are never deleted/recreated or have their tools metadata reset.
		for _, selected := range input.CustomModels {
			exists := false
			for _, row := range rows {
				if row.Name == selected.ModelName {
					exists = true
				}
			}
			if !exists {
				unknownWrite = true
				if _, err = client.AddInstanceModel(ctx, factory, binding.InstanceName, selected); err != nil {
					var rejected interface{ Rejected() bool }
					if errors.As(err, &rejected) && rejected.Rejected() {
						unknownWrite = false
					}
					return nil, err
				}
				unknownWrite = false
			}
		}
		rows, err = client.ListInstanceModels(ctx, factory, binding.InstanceName)
		if err != nil {
			return nil, err
		}
	}
	result.PerCall, err = matchingModels(rows, remote.ID, input)
	if err != nil {
		return nil, err
	}
	if binding.State == "creating" {
		// Atomic creation must contain exactly the selected declarations.
		if len(rows) != len(input.CustomModels) {
			return nil, fmt.Errorf("MultiRAG atomic registration readback differs")
		}
		for _, row := range rows {
			if row.IsTools {
				return nil, fmt.Errorf("MultiRAG created unexpected tools metadata")
			}
		}
	}
	// Keep model declarations out of the credential digest: adding a model does
	// not authorize rotating an instance's credentials or rebinding its identity.
	binding.InstanceID = remote.ID
	binding.ConfigHash = configHash(input)
	sourceErr := s.currentSyncCandidate(tenantID, p.ID(), selectedIDs, factory, input)
	if err = store.ConfirmMultiRAGBinding(tenantID, binding); err != nil {
		return nil, err
	}
	finished = true
	if sourceErr != nil {
		return nil, sourceErr
	}
	result.Success = true
	result.CallCount = len(result.PerCall)
	return result, nil
}
