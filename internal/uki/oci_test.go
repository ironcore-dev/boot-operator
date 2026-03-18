package uki

import (
	"testing"
)

func TestParseOCIReferenceForUKI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		image    string
		wantRepo string
		wantRef  string
		wantErr  bool
	}{
		{
			name:     "registry host port with tag",
			image:    "myregistry:5000/repo/image:v1.0",
			wantRepo: "myregistry:5000/repo/image",
			wantRef:  "myregistry:5000/repo/image:v1.0",
		},
		{
			name:     "simple tag",
			image:    "repo/image:v1.0",
			wantRepo: "repo/image",
			wantRef:  "repo/image:v1.0",
		},
		{
			name:     "digest only",
			image:    "repo/image@sha256:deadbeef",
			wantRepo: "repo/image",
			wantRef:  "repo/image@sha256:deadbeef",
		},
		{
			name:     "host port with digest",
			image:    "myregistry:5000/repo/image@sha256:deadbeef",
			wantRepo: "myregistry:5000/repo/image",
			wantRef:  "myregistry:5000/repo/image@sha256:deadbeef",
		},
		{
			name:     "tag plus digest keeps repo without tag",
			image:    "repo/image:v1.0@sha256:deadbeef",
			wantRepo: "repo/image",
			wantRef:  "repo/image:v1.0@sha256:deadbeef",
		},
		{
			name:    "missing tag and digest",
			image:   "repo/image",
			wantErr: true,
		},
		{
			name:    "empty tag",
			image:   "repo/image:",
			wantErr: true,
		},
		{
			name:    "empty digest",
			image:   "repo/image@",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotRepo, gotRef, err := parseOCIReferenceForUKI(tt.image)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (repo=%q, ref=%q)", gotRepo, gotRef)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotRepo != tt.wantRepo {
				t.Fatalf("repo: got %q, want %q", gotRepo, tt.wantRepo)
			}
			if gotRef != tt.wantRef {
				t.Fatalf("ref: got %q, want %q", gotRef, tt.wantRef)
			}
		})
	}
}
