package domain

import (
	"time"
)

// AMDConfig define os parâmetros em memória de triagem acústica e reconhecimento de voz.
//
// @pattern DTO
// @governedBy docs/rules/TELEPHONY_POLICIES.md
type AMDConfig struct {
	InitialSilenceMs       int       `json:"initial_silence_ms"`
	GreetingMs             int       `json:"greeting_ms"`
	AfterGreetingSilenceMs int       `json:"after_greeting_silence_ms"`
	TotalAnalysisTimeMs    int       `json:"total_analysis_time_ms"`
	MinWordLengthMs        int       `json:"min_word_length_ms"`
	BetweenWordsSilenceMs  int       `json:"between_words_silence_ms"`
	MaximumNumberOfWords   int       `json:"maximum_number_of_words"`
	SilenceThreshold       int       `json:"silence_threshold"`
	MaxSilenceMs           int       `json:"max_silence_ms"`
	VoicemailPhrases       []string  `json:"voicemail_phrases"`
	HumanGreetings         []string  `json:"human_greetings"`
	VoskMaxDurationSec     float64   `json:"vosk_max_duration_sec"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// DefaultAMDConfig retorna os parâmetros balanceados para telefonia brasileira.
func DefaultAMDConfig() AMDConfig {
	return AMDConfig{
		InitialSilenceMs:       2000,
		GreetingMs:             1500,
		AfterGreetingSilenceMs: 600,
		TotalAnalysisTimeMs:    3500,
		MinWordLengthMs:        100,
		BetweenWordsSilenceMs:  50,
		MaximumNumberOfWords:   3,
		SilenceThreshold:       256,
		MaxSilenceMs:           2000,
		VoicemailPhrases: []string{
			"caixa postal",
			"deixe recado",
			"deixe seu recado",
			"recado",
			"assim que possivel",
			"apos o sinal",
			"apos o bip",
			"nao pode atender",
			"nao pode receber chamadas",
			"impossibilitado de atender",
			"esta impossibilitado",
			"nao esta disponivel",
			"chamada encaminhada",
			"secretaria eletronica",
			"mensagem gravada",
			"vivo informa",
			"claro informa",
			"tim informa",
			"desligue a chamada",
			"nao esta recebendo chamadas",
			"caixa de mensagens",
			"sua mensagem",
		},
		HumanGreetings: []string{
			"alo",
			"ola",
			"oi",
			"sim",
			"pronto",
			"pois nao",
			"quem fala",
			"quem e",
			"fala",
			"opa",
			"bom dia",
			"boa tarde",
			"boa noite",
			"pode falar",
			"com quem",
		},
		VoskMaxDurationSec: 2.8,
		UpdatedAt:          time.Now(),
	}
}

// UpdateAMDConfigRequest define o contrato de alteração da configuração do AMD via REST.
type UpdateAMDConfigRequest struct {
	InitialSilenceMs       *int      `json:"initial_silence_ms,omitempty"`
	GreetingMs             *int      `json:"greeting_ms,omitempty"`
	AfterGreetingSilenceMs *int      `json:"after_greeting_silence_ms,omitempty"`
	TotalAnalysisTimeMs    *int      `json:"total_analysis_time_ms,omitempty"`
	MinWordLengthMs        *int      `json:"min_word_length_ms,omitempty"`
	BetweenWordsSilenceMs  *int      `json:"between_words_silence_ms,omitempty"`
	MaximumNumberOfWords   *int      `json:"maximum_number_of_words,omitempty"`
	SilenceThreshold       *int      `json:"silence_threshold,omitempty"`
	MaxSilenceMs           *int      `json:"max_silence_ms,omitempty"`
	VoicemailPhrases       []string  `json:"voicemail_phrases,omitempty"`
	HumanGreetings         []string  `json:"human_greetings,omitempty"`
	VoskMaxDurationSec     *float64  `json:"vosk_max_duration_sec,omitempty"`
	Apply                  bool      `json:"apply"`
}

// AMDConfigResponse envelopa a configuração com status de aplicação no PBX.
type AMDConfigResponse struct {
	Config          AMDConfig `json:"config"`
	AppliedAsterisk bool      `json:"applied_asterisk"`
	Message         string    `json:"message"`
}
