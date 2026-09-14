package main

import (
	"encoding/json"
	"os"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Frases inequívocas de caixas postais, correio de voz e operadoras
var voicemailPhrases = []string{
	"caixa postal",
	"deixe recado",
	"deixe seu recado",
	"recado",
	"assim que possivel",
	"deixe sua mensagem",
	"apos o sinal",
	"apos o bip",
	"nao pode atender",
	"nao pode receber chamadas",
	"impossibilitado de atender",
	"esta impossibilitado",
	"nao esta disponivel",
	"chamada encaminhada",
	"chamada esta sendo",
	"secretaria eletronica",
	"mensagem gravada",
	"horario de atendimento",
	"vivo informa",
	"claro informa",
	"tim informa",
	"numero chamado",
	"o numero para o qual",
	"este numero de telefone",
	"o telefone que voce",
	"desligue a chamada",
	"desligue o telefone",
	"nao esta recebendo chamadas",
	"sua chamada foi completada",
	"programado para nao receber",
	"caixa de mensagens",
	"sua mensagem",
}

// Saudações e confirmações humanas habituais em telefonia brasileira
var humanGreetings = []string{
	"alo",
	"ola",
	"oi",
	"sim",
	"pronto",
	"pois nao",
	"quem fala",
	"quem e",
	"quem ta falando",
	"fala",
	"opa",
	"bom dia",
	"boa tarde",
	"boa noite",
	"pode falar",
	"com quem",
	"aqui e",
	"sou eu",
	"e ele",
	"e ela",
	"ele mesmo",
	"ela mesma",
	"eu mesmo",
	"com ele",
	"com ela",
	"o que deseja",
	"diga",
}

func loadDynamicConfig(maxDuration *float64) {
	for _, p := range []string{"/etc/asterisk/vosk_amd.json", "/opt/ominichat/asterisk/conf/vosk_amd.json", "./storage/vosk_amd.json"} {
		if data, err := os.ReadFile(p); err == nil {
			var cfg struct {
				VoskMaxDurationSec float64  `json:"vosk_max_duration_sec"`
				VoicemailPhrases   []string `json:"voicemail_phrases"`
				HumanGreetings     []string `json:"human_greetings"`
			}
			if json.Unmarshal(data, &cfg) == nil {
				if cfg.VoskMaxDurationSec > 0 {
					*maxDuration = cfg.VoskMaxDurationSec
				}
				if len(cfg.VoicemailPhrases) > 0 {
					voicemailPhrases = cfg.VoicemailPhrases
				}
				if len(cfg.HumanGreetings) > 0 {
					humanGreetings = cfg.HumanGreetings
				}
				break
			}
		}
	}
}

func normalizeText(s string) string {
	if s == "" {
		return ""
	}
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	res, _, err := transform.String(t, s)
	if err != nil {
		res = s
	}
	return strings.ToLower(strings.TrimSpace(res))
}

// ClassifyOutcome analisa o texto acumulado da chamada e determina o status e causa da triagem.
// Silêncio (texto vazio) é rigorosamente classificado como MACHINE com causa VOICEMAIL_SILENCE.
func ClassifyOutcome(fullText string, vmPhrases, greetings []string) (status, cause string) {
	if len(vmPhrases) == 0 {
		vmPhrases = voicemailPhrases
	}
	if len(greetings) == 0 {
		greetings = humanGreetings
	}

	norm := normalizeText(fullText)
	if norm == "" {
		return "MACHINE", "VOICEMAIL_SILENCE"
	}

	for _, phrase := range vmPhrases {
		if strings.Contains(norm, phrase) {
			return "MACHINE", "VOICEMAIL_" + strings.ToUpper(strings.ReplaceAll(phrase, " ", "_"))
		}
	}

	for _, greeting := range greetings {
		if strings.Contains(norm, greeting) {
			return "HUMAN", "HUMAN_" + strings.ToUpper(strings.ReplaceAll(greeting, " ", "_"))
		}
	}

	return "HUMAN", "HUMAN_NATURAL_SPEECH"
}

