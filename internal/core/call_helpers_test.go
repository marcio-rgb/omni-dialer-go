package core

import (
	"testing"
)

func TestCleanAgentID_Sanitization(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Local channel with sala_agente prefix and /n suffix",
			input:    "Local/sala_agente_1b4408a2-f670-4d4b-a790-da50fbc7d1d8@from-internal/n",
			expected: "1b4408a2-f670-4d4b-a790-da50fbc7d1d8",
		},
		{
			name:     "PJSIP channel with UUID",
			input:    "PJSIP/1b4408a2-f670-4d4b-a790-da50fbc7d1d8",
			expected: "1b4408a2-f670-4d4b-a790-da50fbc7d1d8",
		},
		{
			name:     "Clean UUID directly",
			input:    "1b4408a2-f670-4d4b-a790-da50fbc7d1d8",
			expected: "1b4408a2-f670-4d4b-a790-da50fbc7d1d8",
		},
		{
			name:     "Local channel without /n",
			input:    "Local/sala_agente_user_123@context",
			expected: "user_123",
		},
		{
			name:     "Agent with agent_ prefix",
			input:    "agent_operador1",
			expected: "operador1",
		},
		{
			name:     "Empty input",
			input:    "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := cleanAgentID(tc.input)
			if res != tc.expected {
				t.Errorf("cleanAgentID(%q) = %q; expected %q", tc.input, res, tc.expected)
			}
		})
	}
}
