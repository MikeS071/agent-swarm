package rules

import (
	"testing"

	"github.com/MikeS071/agent-swarm/internal/tracker"
)

func TestTicketDescHasScopeAndVerify(t *testing.T) {
	if !TicketDescHasScopeAndVerify("Implement guardian spawn checks. Scope: watchdog spawn path. Verify: go test ./...") {
		t.Fatal("expected ticket description to satisfy scope+verify rule")
	}
	missing := MissingTicketDescFields("Implement guardian spawn checks")
	if len(missing) != 2 {
		t.Fatalf("expected 2 missing fields, got %v", missing)
	}
}

func TestPromptHasRequiredSections(t *testing.T) {
	prompt := `# ticket

## Objective
x

## Dependencies
y

## Scope
z

## Verify
go test ./...
`
	if !PromptHasRequiredSections(prompt) {
		t.Fatal("expected prompt to satisfy required sections")
	}

	missing := MissingPromptSections("# ticket\n\n## Objective\nonly")
	if len(missing) == 0 {
		t.Fatal("expected missing prompt sections")
	}
}

func TestPhaseIntGapTstChainValid(t *testing.T) {
	tickets := map[string]tracker.Ticket{
		"int-g5": {Phase: 3, Type: "int", Depends: nil},
		"gap-g5": {Phase: 3, Type: "gap", Depends: []string{"int-g5"}},
		"tst-g5": {Phase: 3, Type: "tst", Depends: []string{"gap-g5"}},
	}
	res := CheckPhaseIntGapTstChain(tickets, 3)
	if !res.Valid() {
		t.Fatalf("expected valid chain, got %+v", res)
	}
}

func TestPhaseIntGapTstChainMissingKinds(t *testing.T) {
	tickets := map[string]tracker.Ticket{
		"int-g5": {Phase: 3, Type: "int"},
		"tst-g5": {Phase: 3, Type: "tst", Depends: []string{"gap-g5"}},
	}
	res := CheckPhaseIntGapTstChain(tickets, 3)
	if res.Valid() {
		t.Fatalf("expected invalid chain due to missing gap")
	}
	if len(res.MissingKinds) != 1 || res.MissingKinds[0] != "gap" {
		t.Fatalf("expected missing gap, got %+v", res)
	}
}

func TestPhaseIntGapTstChainDependencyOrder(t *testing.T) {
	tickets := map[string]tracker.Ticket{
		"int-g5": {Phase: 3, Type: "int"},
		"gap-g5": {Phase: 3, Type: "gap", Depends: []string{"other-ticket"}},
		"tst-g5": {Phase: 3, Type: "tst", Depends: []string{"gap-g5"}},
	}
	res := CheckPhaseIntGapTstChain(tickets, 3)
	if res.Valid() {
		t.Fatalf("expected invalid chain due to gap missing int dependency")
	}
	if len(res.GapWithoutInt) != 1 || res.GapWithoutInt[0] != "gap-g5" {
		t.Fatalf("expected gap-g5 in GapWithoutInt, got %+v", res)
	}

	tickets["gap-g5"] = tracker.Ticket{Phase: 3, Type: "gap", Depends: []string{"int-g5"}}
	tickets["tst-g5"] = tracker.Ticket{Phase: 3, Type: "tst", Depends: []string{"int-g5"}}
	res = CheckPhaseIntGapTstChain(tickets, 3)
	if res.Valid() {
		t.Fatalf("expected invalid chain due to tst missing gap dependency")
	}
	if len(res.TstWithoutGap) != 1 || res.TstWithoutGap[0] != "tst-g5" {
		t.Fatalf("expected tst-g5 in TstWithoutGap, got %+v", res)
	}
}

func TestPhaseIntGapTstChainInfersKindsFromID(t *testing.T) {
	tickets := map[string]tracker.Ticket{
		"int-alpha": {Phase: 1},
		"gap-alpha": {Phase: 1, Depends: []string{"int-alpha"}},
		"tst-alpha": {Phase: 1, Depends: []string{"gap-alpha"}},
	}
	res := CheckPhaseIntGapTstChain(tickets, 1)
	if !res.Valid() {
		t.Fatalf("expected inferred-id chain to be valid, got %+v", res)
	}
}
