package uki

import (
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
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

func TestFindManifestByArchitecture(t *testing.T) {
	t.Parallel()

	d1 := ocispec.Descriptor{
		Digest: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		Platform: &ocispec.Platform{
			Architecture: "amd64",
			OS:           "linux",
		},
	}
	d2 := ocispec.Descriptor{
		Digest: "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		Platform: &ocispec.Platform{
			Architecture: "arm64",
			OS:           "linux",
		},
	}

	index := ocispec.Index{Manifests: []ocispec.Descriptor{d1, d2}}

	got, ok := findManifestByArchitecture(index, "arm64")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if got.Digest != d2.Digest {
		t.Fatalf("got digest %q, want %q", got.Digest, d2.Digest)
	}

	_, ok = findManifestByArchitecture(index, "ppc64le")
	if ok {
		t.Fatalf("expected ok=false")
	}
}
