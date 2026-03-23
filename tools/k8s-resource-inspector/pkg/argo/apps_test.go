package argo

import (
	"testing"
)

func TestParseMultiSource(t *testing.T) {
	tests := []struct {
		name               string
		sources            []interface{}
		wantRepoURL        string
		wantTargetRevision string
		wantPath           string
		wantValueFiles     []string
		wantOK             bool
	}{
		{
			name: "standard multi-source with values and tags refs",
			sources: []interface{}{
				map[string]interface{}{
					"repoURL":         "oci://registry.example.com/charts",
					"chart":           "my-app",
					"targetRevision":  "1.2.3",
					"helm": map[string]interface{}{
						"valueFiles": []interface{}{
							"$values/apps/my-app/values.yaml",
							"$tags/apps/my-app/image-values.yaml",
						},
					},
				},
				map[string]interface{}{
					"repoURL":        "https://github.com/org/infra",
					"targetRevision": "HEAD",
					"ref":            "values",
				},
				map[string]interface{}{
					"repoURL":        "https://github.com/org/image-tags",
					"targetRevision": "HEAD",
					"ref":            "tags",
				},
			},
			wantRepoURL:        "https://github.com/org/infra",
			wantTargetRevision: "HEAD",
			wantPath:           "apps/my-app",
			wantValueFiles: []string{
				"$values/apps/my-app/values.yaml",
				"$tags/apps/my-app/image-values.yaml",
			},
			wantOK: true,
		},
		{
			name: "values file at repo root",
			sources: []interface{}{
				map[string]interface{}{
					"repoURL": "oci://registry.example.com/charts",
					"chart":   "my-app",
					"helm": map[string]interface{}{
						"valueFiles": []interface{}{
							"$values/values.yaml",
						},
					},
				},
				map[string]interface{}{
					"repoURL":        "https://github.com/org/infra",
					"targetRevision": "main",
					"ref":            "values",
				},
			},
			wantRepoURL:        "https://github.com/org/infra",
			wantTargetRevision: "main",
			wantPath:           "",
			wantValueFiles:     []string{"$values/values.yaml"},
			wantOK:             true,
		},
		{
			name: "missing values source",
			sources: []interface{}{
				map[string]interface{}{
					"repoURL": "oci://registry.example.com/charts",
					"chart":   "my-app",
					"helm": map[string]interface{}{
						"valueFiles": []interface{}{"$values/apps/my-app/values.yaml"},
					},
				},
			},
			wantOK: false,
		},
		{
			name: "missing helm source",
			sources: []interface{}{
				map[string]interface{}{
					"repoURL":        "https://github.com/org/infra",
					"targetRevision": "HEAD",
					"ref":            "values",
				},
			},
			wantOK: false,
		},
		{
			name:   "empty sources",
			sources: []interface{}{},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoURL, targetRevision, appPath, valueFiles, ok := parseMultiSource(tt.sources)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if repoURL != tt.wantRepoURL {
				t.Errorf("repoURL = %q, want %q", repoURL, tt.wantRepoURL)
			}
			if targetRevision != tt.wantTargetRevision {
				t.Errorf("targetRevision = %q, want %q", targetRevision, tt.wantTargetRevision)
			}
			if appPath != tt.wantPath {
				t.Errorf("path = %q, want %q", appPath, tt.wantPath)
			}
			if len(valueFiles) != len(tt.wantValueFiles) {
				t.Fatalf("valueFiles = %v, want %v", valueFiles, tt.wantValueFiles)
			}
			for i, vf := range valueFiles {
				if vf != tt.wantValueFiles[i] {
					t.Errorf("valueFiles[%d] = %q, want %q", i, vf, tt.wantValueFiles[i])
				}
			}
		})
	}
}
