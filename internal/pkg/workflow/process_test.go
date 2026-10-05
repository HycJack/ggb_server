package workflow

import (
	"reflect"
	"testing"
)

func TestSafeUnmarshalWithLatex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
		wantErr  bool
	}{
		{
			name:  "普通JSON",
			input: `{"key":"value"}`,
			expected: map[string]string{
				"key": "value",
			},
			wantErr: false,
		},
		{
			name:  "含Latex公式",
			input: `{"formula":"$\\frac{a}{b}$"}`,
			expected: map[string]string{
				"formula": `$\\frac{a}{b}$`,
			},
			wantErr: false,
		},
		{
			name:  "含Latex公式未转义",
			input: `{"formula":"$\frac{a}{b}$"}`,
			expected: map[string]string{
				"formula": `$\frac{a}{b}$`, // 会被safeUnmarshalWithLatex转成双反斜杠
			},
			wantErr: false,
		},
		{
			name:  "URL字符串",
			input: `{"url":"https://example.com/path"}`,
			expected: map[string]string{
				"url": "https://example.com/path",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := safeUnmarshalWithLatex(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("safeUnmarshalWithLatex() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("safeUnmarshalWithLatex() got = %#v, want %#v", got, tt.expected)
			}
		})
	}
}
