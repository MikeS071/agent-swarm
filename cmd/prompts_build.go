package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/tracker"
)

const (
	compiledPromptMarker    = "<!-- agent-swarm prompts build: v1 -->"
	compiledTicketLayerName = "ticket"
)

type promptBuildLayerInput struct {
	name       string
	sourcePath string
	content    []byte
}

func BuildPrompts(trackerPath, promptDir, repoRoot, specPath string, all bool, args []string) ([]string, error) {
	if strings.TrimSpace(trackerPath) == "" {
		return nil, fmt.Errorf("tracker path is required")
	}
	if strings.TrimSpace(promptDir) == "" {
		return nil, fmt.Errorf("prompt dir is required")
	}
	if strings.TrimSpace(repoRoot) == "" {
		return nil, fmt.Errorf("repo root is required")
	}

	tr, err := tracker.Load(trackerPath)
	if err != nil {
		return nil, err
	}

	ticketIDs, err := selectPromptBuildTickets(tr, all, args)
	if err != nil {
		return nil, err
	}

	for _, ticketID := range ticketIDs {
		tk := tr.Tickets[ticketID]
		if err := buildPromptForTicket(promptDir, repoRoot, specPath, ticketID, tk); err != nil {
			return nil, fmt.Errorf("build prompt %s: %w", ticketID, err)
		}
	}

	return ticketIDs, nil
}

func selectPromptBuildTickets(tr *tracker.Tracker, all bool, args []string) ([]string, error) {
	if tr == nil {
		return nil, fmt.Errorf("tracker is required")
	}

	if (all && len(args) > 0) || (!all && len(args) != 1) {
		return nil, fmt.Errorf("expected exactly one ticket id or --all")
	}

	if all {
		ids := make([]string, 0, len(tr.Tickets))
		for id := range tr.Tickets {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return ids, nil
	}

	ticketID := strings.TrimSpace(args[0])
	if ticketID == "" {
		return nil, fmt.Errorf("ticket id cannot be empty")
	}
	if _, ok := tr.Tickets[ticketID]; !ok {
		return nil, fmt.Errorf("ticket %q not found", ticketID)
	}
	return []string{ticketID}, nil
}

func buildPromptForTicket(promptDir, repoRoot, specPath, ticketID string, tk tracker.Ticket) error {
	ticketPromptPath := filepath.Join(promptDir, ticketID+".md")
	ticketLayerContent, err := readTicketLayerSource(ticketPromptPath)
	if err != nil {
		return err
	}

	layers := make([]promptBuildLayerInput, 0, 5)
	layers, err = appendPromptBuildLayer(layers, "governance", filepath.Join(repoRoot, "AGENTS.md"), false)
	if err != nil {
		return err
	}
	if strings.TrimSpace(specPath) != "" {
		layers, err = appendPromptBuildLayer(layers, "specification", specPath, true)
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(tk.Profile) != "" {
		profilePath := filepath.Join(repoRoot, ".agents", "profiles", strings.TrimSpace(tk.Profile)+".md")
		layers, err = appendPromptBuildLayer(layers, "profile", profilePath, true)
		if err != nil {
			return fmt.Errorf("profile context unresolved: %w", err)
		}
	}
	layers = append(layers, promptBuildLayerInput{
		name:       compiledTicketLayerName,
		sourcePath: ticketPromptPath,
		content:    ticketLayerContent,
	})

	footerPath := filepath.Join(filepath.Dir(promptDir), "prompt-footer.md")
	layers, err = appendPromptBuildLayer(layers, "footer", footerPath, false)
	if err != nil {
		return err
	}

	compiled := renderCompiledPrompt(ticketID, layers)
	if err := os.WriteFile(ticketPromptPath, compiled, 0o644); err != nil {
		return fmt.Errorf("write compiled prompt %s: %w", ticketPromptPath, err)
	}

	manifest := buildPromptManifest(ticketID, repoRoot, ticketPromptPath, compiled, layers)
	manifestBytes, err := marshalPromptBuildManifest(manifest)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(promptDir, ticketID+".manifest.json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		return fmt.Errorf("write prompt manifest %s: %w", manifestPath, err)
	}

	return nil
}

func appendPromptBuildLayer(layers []promptBuildLayerInput, name, sourcePath string, required bool) ([]promptBuildLayerInput, error) {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		if os.IsNotExist(err) && !required {
			return layers, nil
		}
		return nil, fmt.Errorf("read %s layer %s: %w", name, sourcePath, err)
	}
	layers = append(layers, promptBuildLayerInput{
		name:       name,
		sourcePath: sourcePath,
		content:    normalizePromptBytes(content),
	})
	return layers, nil
}

func readTicketLayerSource(ticketPromptPath string) ([]byte, error) {
	data, err := os.ReadFile(ticketPromptPath)
	if err != nil {
		return nil, fmt.Errorf("read ticket prompt %s: %w", ticketPromptPath, err)
	}
	if content, ok := extractCompiledLayer(data, compiledTicketLayerName); ok {
		return normalizePromptBytes(content), nil
	}
	return normalizePromptBytes(data), nil
}

func extractCompiledLayer(compiled []byte, layerName string) ([]byte, bool) {
	if !bytes.HasPrefix(compiled, []byte(compiledPromptMarker)) {
		return nil, false
	}

	text := string(compiled)
	startToken := "<!-- layer:" + layerName + " -->"
	endToken := "<!-- endlayer:" + layerName + " -->"
	start := strings.Index(text, startToken)
	if start < 0 {
		return nil, false
	}
	start += len(startToken)
	for start < len(text) && (text[start] == '\n' || text[start] == '\r') {
		start++
	}
	end := strings.Index(text[start:], endToken)
	if end < 0 {
		return nil, false
	}
	end += start
	return []byte(text[start:end]), true
}

func renderCompiledPrompt(ticketID string, layers []promptBuildLayerInput) []byte {
	var b strings.Builder
	b.WriteString(compiledPromptMarker)
	b.WriteString("\n")
	b.WriteString("<!-- ticket:")
	b.WriteString(ticketID)
	b.WriteString(" -->\n\n")

	for _, layer := range layers {
		b.WriteString("<!-- layer:")
		b.WriteString(layer.name)
		b.WriteString(" -->\n")
		b.Write(layer.content)
		if !bytes.HasSuffix(layer.content, []byte("\n")) {
			b.WriteString("\n")
		}
		b.WriteString("<!-- endlayer:")
		b.WriteString(layer.name)
		b.WriteString(" -->\n\n")
	}
	return []byte(b.String())
}

func normalizePromptBytes(data []byte) []byte {
	normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if len(normalized) == 0 || normalized[len(normalized)-1] == '\n' {
		return normalized
	}
	return append(normalized, '\n')
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
