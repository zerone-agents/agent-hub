package extensionmanifest

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

//go:embed schema/agenthub-extension-v1alpha1.schema.json
var schemaDocument []byte

type Report struct {
	Manifest string
	Valid    bool
	Errors   []string
}

func ValidatePackage(packageDir string) Report {
	manifestPath := filepath.Join(packageDir, "extension.yaml")
	report := Report{Manifest: manifestPath}
	packageRoot, err := filepath.EvalSymlinks(packageDir)
	if err != nil {
		report.Errors = []string{fmt.Sprintf("resolve package directory: %v", err)}
		return report
	}
	packageRoot, err = filepath.Abs(packageRoot)
	if err != nil {
		report.Errors = []string{fmt.Sprintf("resolve package directory: %v", err)}
		return report
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		report.Errors = []string{fmt.Sprintf("read manifest: %v", err)}
		return report
	}

	var document any
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&document); err != nil {
		report.Errors = []string{fmt.Sprintf("parse YAML: %v", err)}
		return report
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		report.Errors = []string{"parse YAML: extension.yaml must contain exactly one document"}
		return report
	} else if !errors.Is(err, io.EOF) {
		report.Errors = []string{fmt.Sprintf("parse YAML: trailing content: %v", err)}
		return report
	}

	compiler := jsonschema.NewCompiler()
	var schemaJSON any
	if err := json.Unmarshal(schemaDocument, &schemaJSON); err != nil {
		report.Errors = []string{fmt.Sprintf("decode embedded schema: %v", err)}
		return report
	}
	if err := compiler.AddResource("agenthub-extension-v1alpha1.schema.json", schemaJSON); err != nil {
		report.Errors = []string{fmt.Sprintf("load embedded schema: %v", err)}
		return report
	}
	schema, err := compiler.Compile("agenthub-extension-v1alpha1.schema.json")
	if err != nil {
		report.Errors = []string{fmt.Sprintf("compile embedded schema: %v", err)}
		return report
	}
	if err := schema.Validate(document); err != nil {
		report.Errors = flattenValidationError(err)
		return report
	}

	jsonBytes, _ := json.Marshal(document)
	var manifest manifestFiles
	if err := json.Unmarshal(jsonBytes, &manifest); err != nil {
		report.Errors = []string{fmt.Sprintf("decode validated manifest: %v", err)}
		return report
	}
	for _, ref := range manifest.Contributes.allFiles() {
		resolved := filepath.Join(packageDir, filepath.FromSlash(ref))
		resolved, err := filepath.EvalSymlinks(resolved)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("referenced file %q: %v", ref, err))
			continue
		}
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("referenced file %q: %v", ref, err))
			continue
		}
		relative, err := filepath.Rel(packageRoot, resolved)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			report.Errors = append(report.Errors, fmt.Sprintf("referenced file %q resolves outside the package", ref))
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("referenced file %q: %v", ref, err))
			continue
		}
		if !info.Mode().IsRegular() {
			report.Errors = append(report.Errors, fmt.Sprintf("referenced file %q is not a regular file", ref))
		}
	}

	report.Valid = len(report.Errors) == 0
	return report
}

func flattenValidationError(err error) []string {
	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		return []string{err.Error()}
	}
	var output []string
	var walk func(*jsonschema.ValidationError)
	walk = func(current *jsonschema.ValidationError) {
		if len(current.Causes) == 0 {
			location := "/" + strings.Join(current.InstanceLocation, "/")
			output = append(output, fmt.Sprintf("%s: %s", location, current.Error()))
			return
		}
		for _, cause := range current.Causes {
			walk(cause)
		}
	}
	walk(validationErr)
	sort.Strings(output)
	return output
}

type manifestFiles struct {
	Contributes contributionFiles `json:"contributes"`
}

type contributionFiles struct {
	StateSchemas    []fileRef `json:"stateSchemas"`
	Events          []fileRef `json:"events"`
	Tools           []fileRef `json:"tools"`
	RelationTypes   []fileRef `json:"relationTypes"`
	PromptFragments []fileRef `json:"promptFragments"`
	UIViews         []fileRef `json:"uiViews"`
	Templates       []fileRef `json:"templates"`
	Handlers        []fileRef `json:"handlers"`
}

type fileRef struct {
	File string `json:"file"`
}

func (c contributionFiles) allFiles() []string {
	groups := [][]fileRef{c.StateSchemas, c.Events, c.Tools, c.RelationTypes, c.PromptFragments, c.UIViews, c.Templates, c.Handlers}
	var files []string
	for _, group := range groups {
		for _, ref := range group {
			if strings.TrimSpace(ref.File) != "" {
				files = append(files, ref.File)
			}
		}
	}
	return files
}
