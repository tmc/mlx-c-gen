package hooks

import (
	"bytes"
	"strings"
	"testing"
)

func TestGraphUtilsHooks(t *testing.T) {
	tests := []struct {
		name     string
		funcName string
		impl     bool
		want     []string
	}{
		{
			name:     "export header",
			funcName: "mlx_export_to_dot",
			want: []string{
				"typedef struct mlx_node_namer_",
				"mlx_node_namer mlx_node_namer_new()",
				"int mlx_export_to_dot(",
			},
		},
		{
			name:     "export implementation",
			funcName: "mlx_export_to_dot",
			impl:     true,
			want: []string{
				"extern \"C\" mlx_node_namer mlx_node_namer_new()",
				"extern \"C\" int mlx_export_to_dot(",
				"CFileOutputStream::as_lvalue(CFileOutputStream(os))",
			},
		},
		{
			name:     "print header",
			funcName: "mlx_print_graph",
			want:     []string{"int mlx_print_graph("},
		},
		{
			name:     "print implementation",
			funcName: "mlx_print_graph",
			impl:     true,
			want: []string{
				"extern \"C\" int mlx_print_graph(",
				"CFileOutputStream::as_lvalue(CFileOutputStream(os))",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := GetHook(tt.funcName)
			if hook == nil {
				t.Fatalf("GetHook(%q) = nil", tt.funcName)
			}
			var buf bytes.Buffer
			if !hook(&buf, tt.funcName, tt.impl) {
				t.Fatalf("hook(%q) returned false", tt.funcName)
			}
			got := buf.String()
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("hook output missing %q\n%s", want, got)
				}
			}
		})
	}
}
