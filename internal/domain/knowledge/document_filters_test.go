package knowledge

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExactMetadataRejectsSilentlyDroppedValues(t *testing.T) {
	for _, raw := range []string{
		`{"field":null}`, `{"field":[null]}`, `{"field":["v2",null]}`, `{"field":[]}`, `{"field":""}`, `{"field":[" \t "]}`,
		`{"field":[1e309]}`, `{"field":[NaN]}`, `{"field":[Infinity]}`,
	} {
		err := ValidateDocumentFilters(DocumentListRequest{Metadata: raw})
		require.Error(t, err, raw)
		require.Equal(t, 400, StatusCode(err), raw)
	}
	for _, raw := range []string{
		`{}`, `{"__proto__":["business-value"],"constructor":["v2"]}`, `{"field":[false,0,true,1,"v2"]}`, `{"field":["NaN","Infinity"]}`,
	} {
		require.NoError(t, ValidateDocumentFilters(DocumentListRequest{Metadata: raw}), raw)
	}
}

func TestExactMetadataPythonControlWhitespaceFailsClosed(t *testing.T) {
	for _, r := range []rune{'\u001c', '\u001d', '\u001e', '\u001f'} {
		for _, value := range []any{string(r), []any{string(r)}, []any{"v2", " \t" + string(r) + "\n"}} {
			data, err := json.Marshal(map[string]any{"field": value})
			require.NoError(t, err)
			err = ValidateDocumentFilters(DocumentListRequest{Metadata: string(data)})
			require.Error(t, err)
			require.Equal(t, 400, StatusCode(err))
		}
		data, err := json.Marshal(map[string]any{"field": []any{string(r) + "Alice" + string(r)}})
		require.NoError(t, err)
		require.NoError(t, ValidateDocumentFilters(DocumentListRequest{Metadata: string(data)}))
	}
}
