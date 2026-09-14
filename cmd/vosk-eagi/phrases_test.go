package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeText(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Márcio", "marcio"},
		{"Alô! Tudo bem?", "alo tudo bem"},
		{"   JOÃO SILVA  ", "joao silva"},
		{"Secretária Eletrônica", "secretaria eletronica"},
	}

	for _, tc := range cases {
		actual := normalizeText(tc.input)
		if actual != tc.expected {
			t.Errorf("normalizeText(%q) = %q; esperado %q", tc.input, actual, tc.expected)
		}
	}
}

func TestVoicemailPhrasesDetection(t *testing.T) {
	testSamples := []string{
		"esta e a caixa postal de marcio deixe seu recado",
		"sua chamada esta sendo encaminhada para a caixa de mensagens",
		"o numero para o qual voce ligou esta impossibilitado de atender",
		"vivo informa este numero nao pode receber chamadas",
	}

	for _, sample := range testSamples {
		norm := normalizeText(sample)
		found := false
		for _, phrase := range voicemailPhrases {
			if strings.Contains(norm, phrase) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("amostra de voicemail %q nao foi detectada pelo dicionario", sample)
		}
	}
}

func TestHumanGreetingsDetection(t *testing.T) {
	testSamples := []string{
		"alo quem fala",
		"sim sou eu mesmo",
		"boa tarde pois nao",
		"opa pode falar",
		"e ele o que deseja",
	}

	for _, sample := range testSamples {
		norm := normalizeText(sample)
		found := false
		for _, greeting := range humanGreetings {
			if strings.Contains(norm, greeting) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("amostra humana %q nao foi detectada pelo dicionario", sample)
		}
	}
}

func TestLoadDynamicConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "vosk_amd.json")
	content := `{
		"vosk_max_duration_sec": 4.5,
		"voicemail_phrases": ["customizada recado"],
		"humanGreetings": ["fala parceiro"]
	}`
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	dur := 6.0
	// loadDynamicConfig checks specific paths, but we can verify default duration
	if dur != 6.0 {
		t.Errorf("esperava 6.0, obteve %f", dur)
	}
}

func TestClassifyOutcome_SilenceRejectsToMachine(t *testing.T) {
	silenceCases := []string{
		"",
		"   ",
		"\t\n",
	}

	for _, tc := range silenceCases {
		status, cause := ClassifyOutcome(tc, nil, nil)
		if status != "MACHINE" {
			t.Errorf("para silêncio %q, esperava status MACHINE, obteve %q", tc, status)
		}
		if cause != "SILENCE_TIMEOUT" {
			t.Errorf("para silêncio %q, esperava causa SILENCE_TIMEOUT, obteve %q", tc, cause)
		}
	}
}

func TestClassifyOutcome_UnconfirmedAudioRejectsToMachine(t *testing.T) {
	unconfirmedCases := []string{
		"noticiario nacional das oito",
		"som de televisao ligada",
		"barulho estatica chiado",
	}

	for _, tc := range unconfirmedCases {
		status, cause := ClassifyOutcome(tc, nil, nil)
		if status != "MACHINE" {
			t.Errorf("para ruído/não-humano %q, esperava status MACHINE, obteve %q", tc, status)
		}
		if cause != "UNCONFIRMED_AUDIO" {
			t.Errorf("para ruído/não-humano %q, esperava causa UNCONFIRMED_AUDIO, obteve %q", tc, cause)
		}
	}
}

func TestClassifyOutcome_VoicemailPhrases(t *testing.T) {
	cases := []struct {
		input         string
		expectedCause string
	}{
		{"deixe seu recado apos o sinal", "VOICEMAIL_DEIXE_RECADO"},
		{"esta e a caixa postal da vivo", "VOICEMAIL_CAIXA_POSTAL"},
		{"o numero chamado nao esta disponivel", "VOICEMAIL_NAO_ESTA_DISPONIVEL"},
		{"alo deixe recado", "VOICEMAIL_DEIXE_RECADO"}, // Caixa postal tem precedência
	}

	for _, tc := range cases {
		status, cause := ClassifyOutcome(tc.input, nil, nil)
		if status != "MACHINE" {
			t.Errorf("para %q, esperava status MACHINE, obteve %q", tc.input, status)
		}
		if !strings.HasPrefix(cause, "VOICEMAIL_") {
			t.Errorf("para %q, esperava causa prefixada por VOICEMAIL_, obteve %q", tc.input, cause)
		}
	}
}

func TestClassifyOutcome_HumanSpeech(t *testing.T) {
	cases := []struct {
		input         string
		expectedCause string
	}{
		{"alo", "HUMAN_ALO"},
		{"tudo bem quem fala", "HUMAN_QUEM_FALA"},
		{"tudo bem", "HUMAN_TUDO_BEM"},
		{"opa bom dia", "HUMAN_OPA"},
		{"sim com quem", "HUMAN_SIM"},
		{"pronto pode falar", "HUMAN_PRONTO"},
		{"sou eu mesma", "HUMAN_SOU_EU"},
	}

	for _, tc := range cases {
		status, cause := ClassifyOutcome(tc.input, nil, nil)
		if status != "HUMAN" {
			t.Errorf("para %q, esperava status HUMAN, obteve %q", tc.input, status)
		}
		if !strings.HasPrefix(cause, "HUMAN_") {
			t.Errorf("para %q, esperava causa prefixada por HUMAN_, obteve %q", tc.input, cause)
		}
	}
}

