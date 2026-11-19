package main

const (
	lyreArrangeStandard = "standard"
	lyreArrangeCreative = "creative"
	lyreArrangeAgent = "agentpro"
)

func normalizeLyreArrangeMode(mode string) string {
	switch mode {
	case lyreArrangeCreative, lyreArrangeAgent:
		return mode
	default:
		return lyreArrangeStandard
	}
}
func lyreArrangeDisplay(mode string) string {
	switch normalizeLyreArrangeMode(mode) {
	case lyreArrangeCreative:
		return "Creative AI"
	case lyreArrangeAgent:
		return "Agent Pro"
	default:
		return "Standard"
	}
}

// Budget is a per-song request guard, not a platform billing limit.
const defaultAgentBudgetCents = 100

func normalizeAgentBudget(cents int) int {
	switch cents {
	case 50, 100, 200:
		return cents
	default:
		return defaultAgentBudgetCents
	}
}
func nextAgentBudget(cents int) int {
	switch normalizeAgentBudget(cents) {
	case 50:
		return 100
	case 100:
		return 200
	default:
		return 50
	}
}
