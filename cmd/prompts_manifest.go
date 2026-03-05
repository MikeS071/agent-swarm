package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type promptBuildManifest struct {
	TicketID   string                     `json:"ticket_id"`
	OutputPath string                     `json:"output_path"`
	OutputSHA  string                     `json:"output_sha256"`
	Layers     []promptBuildManifestLayer `json:"layers"`
}

type promptBuildManifestLayer struct {
	Name       string `json:"name"`
	SourcePath string `json:"source_path,omitempty"`
	SHA256     string `json:"sha256"`
}

func buildPromptManifest(ticketID, repoRoot, outputPath string, compiled []byte, layers []promptBuildLayerInput) promptBuildManifest {
	manifestLayers := make([]promptBuildManifestLayer, 0, len(layers))
	for _, layer := range layers {
		manifestLayers = append(manifestLayers, promptBuildManifestLayer{
			Name:       layer.name,
			SourcePath: manifestPath(repoRoot, layer.sourcePath),
			SHA256:     sha256Hex(layer.content),
		})
	}
	return promptBuildManifest{
		TicketID:   ticketID,
		OutputPath: manifestPath(repoRoot, outputPath),
		OutputSHA:  sha256Hex(compiled),
		Layers:     manifestLayers,
	}
}

func marshalPromptBuildManifest(manifest promptBuildManifest) ([]byte, error) {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal prompt manifest for %s: %w", manifest.TicketID, err)
	}
	return append(data, '\n'), nil
}

func manifestPath(repoRoot, path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	if strings.TrimSpace(repoRoot) != "" {
		if rel, err := filepath.Rel(repoRoot, path); err == nil {
			rel = filepath.ToSlash(filepath.Clean(rel))
			if rel != ".." && !strings.HasPrefix(rel, "../") {
				return rel
			}
		}
	}
	return filepath.ToSlash(filepath.Clean(path))
}
