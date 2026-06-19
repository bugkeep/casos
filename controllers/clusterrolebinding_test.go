package controllers

import "testing"

func TestNormalizeClusterRoleBindingRoleRefKind(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "empty defaults to cluster role",
			input: "",
			want:  "ClusterRole",
		},
		{
			name:  "cluster role is accepted",
			input: "ClusterRole",
			want:  "ClusterRole",
		},
		{
			name:    "role is rejected",
			input:   "Role",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeClusterRoleBindingRoleRefKind(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
