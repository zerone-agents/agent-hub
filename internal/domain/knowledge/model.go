package knowledge

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Object is a JSON object carried across the knowledge gateway boundary.
// The multirag API is still evolving, so the domain layer keeps stable field
// names while preserving additional fields returned by the remote service.
type Object map[string]any

type Dataset Object
type Document Object
type Chunk Object

type DatasetListRequest struct {
	ID                   string
	Name                 string
	Page                 int
	PageSize             int
	OrderBy              string
	Desc                 *bool
	Keywords             string
	ParserID             string
	OwnerIDs             []string
	IncludeParsingStatus bool
}

type DatasetMutationRequest Object

type DatasetListResult struct {
	Total    int       `json:"total"`
	Datasets []Dataset `json:"datasets"`
}

type DeleteRequest struct {
	IDs       []string `json:"ids,omitempty"`
	DeleteAll bool     `json:"delete_all,omitempty"`
}

type DocumentListRequest struct {
	Page              int
	PageSize          int
	OrderBy           string
	Desc              *bool
	Keywords          string
	ID                string
	Name              string
	Suffix            []string
	Run               []string
	CreateTimeFrom    int64
	CreateTimeTo      int64
	MetadataCondition string
}

type DocumentUpdateRequest Object

type DocumentListResult struct {
	Total     int        `json:"total"`
	Documents []Document `json:"documents"`
}

type UploadRequest struct {
	Body          io.Reader
	ContentType   string
	ContentLength int64
}

type StreamResult struct {
	Body               io.ReadCloser
	ContentType        string
	ContentDisposition string
	ContentLength      int64
}

type ParseDocumentsRequest struct {
	DocumentIDs []string `json:"document_ids"`
}

type ChunkListRequest struct {
	Page      int
	PageSize  int
	Keywords  string
	ID        string
	Available *bool
}

type ChunkMutationRequest Object

type DeleteChunksRequest struct {
	ChunkIDs  []string `json:"chunk_ids,omitempty"`
	DeleteAll bool     `json:"delete_all,omitempty"`
}

type ChunkListResult struct {
	Total    int      `json:"total"`
	Chunks   []Chunk  `json:"chunks"`
	Document Document `json:"document,omitempty"`
}

type RetrievalRequest Object

type RetrievalResult Object

type HealthStatus struct {
	Configured bool   `json:"configured"`
	Connected  bool   `json:"connected"`
	Status     string `json:"status"`
	Message    string `json:"message"`
}

type ErrorKind string

const (
	ErrorKindUnavailable ErrorKind = "unavailable"
	ErrorKindBadRequest  ErrorKind = "bad_request"
	ErrorKindNotFound    ErrorKind = "not_found"
	ErrorKindUpstream    ErrorKind = "upstream"
)

type Error struct {
	Kind       ErrorKind
	Message    string
	HTTPStatus int
	Cause      error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NewUnavailableError(message string) *Error {
	return &Error{Kind: ErrorKindUnavailable, Message: message, HTTPStatus: http.StatusServiceUnavailable}
}

func NewBadRequestError(message string) *Error {
	return &Error{Kind: ErrorKindBadRequest, Message: message, HTTPStatus: http.StatusBadRequest}
}

func NewNotFoundError(message string) *Error {
	return &Error{Kind: ErrorKindNotFound, Message: message, HTTPStatus: http.StatusNotFound}
}

func NewUpstreamError(message string, cause error) *Error {
	return &Error{Kind: ErrorKindUpstream, Message: message, HTTPStatus: http.StatusBadGateway, Cause: cause}
}

func StatusCode(err error) int {
	var knowledgeErr *Error
	if errors.As(err, &knowledgeErr) && knowledgeErr.HTTPStatus != 0 {
		return knowledgeErr.HTTPStatus
	}
	return http.StatusInternalServerError
}

var InboundDatasetKeyMap = map[string]string{
	"document_count":  "doc_num",
	"chunk_count":     "chunk_num",
	"chunk_method":    "parser_id",
	"embedding_model": "embd_id",
}

var OutboundDatasetKeyMap = map[string]string{
	"parser_id": "chunk_method",
	"embd_id":   "embedding_model",
}

const datasetControlPanelConfigKey = "control_panel"

var datasetCollectionNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,254}$`)

var datasetCreateParserConfigKeys = map[string]struct{}{
	"auto_keywords":        {},
	"auto_questions":       {},
	"chunk_token_num":      {},
	"delimiter":            {},
	"filename_embd_weight": {},
	"html4excel":           {},
	"layout_recognize":     {},
	"pages":                {},
	"tag_kb_ids":           {},
	"task_page_size":       {},
	"topn_tags":            {},
}

var datasetRaptorConfigKeys = map[string]struct{}{
	"auto_disable_for_structured_data": {},
	"max_cluster":                      {},
	"max_token":                        {},
	"prompt":                           {},
	"random_seed":                      {},
	"threshold":                        {},
	"use_raptor":                       {},
}

var datasetGraphragConfigKeys = map[string]struct{}{
	"community":    {},
	"entity_types": {},
	"method":       {},
	"resolution":   {},
	"use_graphrag": {},
}

var datasetParentChildConfigKeys = map[string]struct{}{
	"children_delimiter": {},
	"use_parent_child":   {},
}

var InboundDocumentKeyMap = map[string]string{
	"chunk_count":  "chunk_num",
	"chunk_method": "parser_id",
	"token_count":  "token_num",
}

var InboundChunkKeyMap = map[string]string{
	"chunk_id":            "id",
	"content_with_weight": "content",
	"doc_id":              "document_id",
	"important_kwd":       "important_keywords",
	"question_kwd":        "questions",
	"img_id":              "image_id",
	"position_int":        "positions",
	"doc_type_kwd":        "doc_type",
}

func NormalizeDataset(raw map[string]any) Dataset {
	dst := renameInbound(raw, InboundDatasetKeyMap)
	physicalName := firstNonEmptyString(dst["collection_name"], raw["collection_name"], raw["name"])
	displayName := datasetDisplayName(dst)
	if physicalName != "" {
		dst["collection_name"] = physicalName
	}
	if displayName != "" {
		dst["display_name"] = displayName
		dst["name"] = displayName
	}
	return Dataset(dst)
}

func NormalizeDocument(raw map[string]any) Document {
	return Document(renameInbound(raw, InboundDocumentKeyMap))
}

func NormalizeChunk(raw map[string]any) Chunk {
	return Chunk(renameInbound(raw, InboundChunkKeyMap))
}

func DatasetMutationToRemote(req DatasetMutationRequest) map[string]any {
	return renameOutbound(map[string]any(req), OutboundDatasetKeyMap)
}

func DatasetCreateToRemote(req DatasetMutationRequest) map[string]any {
	remote := DatasetMutationToRemote(req)
	remote["name"] = datasetCollectionName(remote)
	if config := strictDatasetCreateParserConfig(remote["parser_config"]); len(config) > 0 {
		remote["parser_config"] = config
	} else {
		delete(remote, "parser_config")
	}
	delete(remote, "display_name")
	delete(remote, "collection_name")
	return remote
}

func DatasetUpdateToRemote(req DatasetMutationRequest) map[string]any {
	remote := DatasetMutationToRemote(req)
	displayName := firstNonEmptyString(remote["display_name"], remote["name"])
	if displayName != "" {
		remote["parser_config"] = withDatasetDisplayName(remote["parser_config"], displayName)
	}
	if config := normalizeDatasetUpdateParserConfig(remote["parser_config"]); len(config) > 0 {
		remote["parser_config"] = config
	} else {
		delete(remote, "parser_config")
	}
	delete(remote, "name")
	delete(remote, "display_name")
	delete(remote, "collection_name")
	return remote
}

func DatasetMutationDisplayName(req DatasetMutationRequest) string {
	return strings.TrimSpace(firstNonEmptyString(req["display_name"], req["name"]))
}

func DocumentUpdateToRemote(req DocumentUpdateRequest) map[string]any {
	remote := renameOutbound(map[string]any(req), map[string]string{
		"parser_id": "chunk_method",
		"chunk_num": "chunk_count",
		"token_num": "token_count",
	})
	if v, ok := remote["enabled"]; ok {
		remote["enabled"] = v
	}
	return remote
}

func ChunkMutationToRemote(req ChunkMutationRequest) map[string]any {
	return renameOutbound(map[string]any(req), map[string]string{
		"important_keywords": "important_keywords",
	})
}

func CloneObject(src map[string]any) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func datasetCollectionName(src map[string]any) string {
	if collectionName := strings.TrimSpace(firstNonEmptyString(src["collection_name"])); datasetCollectionNamePattern.MatchString(collectionName) {
		return collectionName
	}
	return generateDatasetCollectionName()
}

func generateDatasetCollectionName() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return "kb_" + hex.EncodeToString(bytes[:])
	}
	return fmt.Sprintf("kb_%d", time.Now().UnixNano())
}

func datasetDisplayName(src map[string]any) string {
	if displayName := strings.TrimSpace(firstNonEmptyString(src["display_name"])); displayName != "" {
		return displayName
	}
	if displayName := displayNameFromParserConfig(src["parser_config"]); displayName != "" {
		return displayName
	}
	return strings.TrimSpace(firstNonEmptyString(src["name"]))
}

func displayNameFromParserConfig(value any) string {
	config, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	controlPanel, ok := config[datasetControlPanelConfigKey].(map[string]any)
	if !ok {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(controlPanel["display_name"]))
}

func withDatasetDisplayName(value any, displayName string) map[string]any {
	config := map[string]any{}
	if existing, ok := value.(map[string]any); ok {
		config = CloneObject(existing)
	}
	controlPanel := map[string]any{}
	if existing, ok := config[datasetControlPanelConfigKey].(map[string]any); ok {
		controlPanel = CloneObject(existing)
	}
	controlPanel["display_name"] = displayName
	config[datasetControlPanelConfigKey] = controlPanel
	return config
}

func strictDatasetCreateParserConfig(value any) map[string]any {
	config, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	dst := make(map[string]any, len(config))
	for key, item := range config {
		if _, ok := datasetCreateParserConfigKeys[key]; ok {
			dst[key] = item
		}
	}
	if raptor := sanitizeNestedParserConfig(config["raptor"], datasetRaptorConfigKeys); len(raptor) > 0 {
		if _, ok := raptor["use_raptor"]; !ok {
			if enabled, ok := configBoolAlias(config["raptor"], "enabled"); ok {
				raptor["use_raptor"] = enabled
			}
		}
		dst["raptor"] = raptor
	} else if enabled, ok := configBoolAlias(config["raptor"], "enabled"); ok {
		dst["raptor"] = map[string]any{"use_raptor": enabled}
	}
	if graphrag := sanitizeNestedParserConfig(config["graphrag"], datasetGraphragConfigKeys); len(graphrag) > 0 {
		if _, ok := graphrag["use_graphrag"]; !ok {
			if enabled, ok := configBoolAlias(config["graphrag"], "enabled"); ok {
				graphrag["use_graphrag"] = enabled
			}
		}
		dst["graphrag"] = graphrag
	} else if enabled, ok := configBoolAlias(config["graphrag"], "enabled"); ok {
		dst["graphrag"] = map[string]any{"use_graphrag": enabled}
	}
	if parentChild := parentChildParserConfig(config); len(parentChild) > 0 {
		dst["parent_child"] = parentChild
	}
	return dst
}

func normalizeDatasetUpdateParserConfig(value any) map[string]any {
	config, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	dst := CloneObject(config)
	if parentChild := parentChildParserConfig(config); len(parentChild) > 0 {
		dst["parent_child"] = parentChild
		delete(dst, "enable_children")
		delete(dst, "children_delimiter")
	}
	normalizeUseFlagAlias(dst, "raptor", "enabled", "use_raptor")
	normalizeUseFlagAlias(dst, "graphrag", "enabled", "use_graphrag")
	return dst
}

func sanitizeNestedParserConfig(value any, allowed map[string]struct{}) map[string]any {
	src, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, item := range src {
		if _, ok := allowed[key]; ok {
			dst[key] = item
		}
	}
	return dst
}

func parentChildParserConfig(config map[string]any) map[string]any {
	parentChild := sanitizeNestedParserConfig(config["parent_child"], datasetParentChildConfigKeys)
	if parentChild == nil {
		parentChild = map[string]any{}
	}
	if enabled, ok := config["enable_children"]; ok {
		parentChild["use_parent_child"] = enabled
	}
	if delimiter, ok := config["children_delimiter"]; ok {
		parentChild["children_delimiter"] = delimiter
	}
	if len(parentChild) == 0 {
		return nil
	}
	return parentChild
}

func normalizeUseFlagAlias(config map[string]any, objectKey string, aliasKey string, targetKey string) {
	nested, ok := config[objectKey].(map[string]any)
	if !ok {
		return
	}
	if _, ok := nested[targetKey]; ok {
		return
	}
	if value, ok := nested[aliasKey]; ok {
		copy := CloneObject(nested)
		copy[targetKey] = value
		delete(copy, aliasKey)
		config[objectKey] = copy
	}
}

func configBoolAlias(value any, key string) (any, bool) {
	config, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	item, ok := config[key]
	return item, ok
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if text, ok := value.(string); ok {
			text = strings.TrimSpace(text)
			if text != "" {
				return text
			}
		}
	}
	return ""
}

func renameInbound(src map[string]any, keyMap map[string]string) map[string]any {
	dst := make(map[string]any, len(src))
	for key, value := range src {
		if newKey, ok := keyMap[key]; ok {
			dst[newKey] = value
			continue
		}
		dst[key] = value
	}
	return dst
}

func renameOutbound(src map[string]any, keyMap map[string]string) map[string]any {
	dst := make(map[string]any, len(src))
	for key, value := range src {
		if newKey, ok := keyMap[key]; ok {
			dst[newKey] = value
			continue
		}
		dst[key] = value
	}
	return dst
}
