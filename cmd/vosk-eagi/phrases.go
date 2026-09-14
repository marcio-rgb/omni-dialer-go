package main

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// CallMetrics encapsula os dados auditivos e textuais da chamada
type CallMetrics struct {
	FullText          string  // Transcrição acumulada do Vosk
	SpeechDurationSec float64 // Tempo total que a pessoa/máquina passou falando
	SilenceAfterSec   float64 // Silêncio detectado após a fala
}

// Termos estritos e inequívocos (sem termos de palavra única perigosos como "recado")
var highConfidenceVM = []string{
	"caixa postal",
	"deixe seu recado",
	"grave seu recado",
	"deixe sua mensagem",
	"apos o sinal",
	"apos o bip",
	"chamada encaminhada",
	"chamada esta sendo encaminhada",
	"secretaria eletronica",
	"mensagem gravada",
	"horario de atendimento",
	"vivo informa",
	"claro informa",
	"tim informa",
	"nao pode receber chamadas",
	"temporariamente fora de servico",
	"este numero de telefone nao",
	"o numero discado nao existe",
	"programado para nao receber",
	"caixa de mensagens",
}

// Saudações humanas autênticas (fala rápida de abertura)
var quickHumanGreetings = []string{
	"alo",
	"ola",
	"oi",
	"pronto",
	"pois nao",
	"quem fala",
	"quem e",
	"quem ta falando",
	"com quem",
	"com quem falo",
	"com quem eu falo",
	"fala",
	"fala ai",
	"opa",
	"bom dia",
	"boa tarde",
	"boa noite",
	"pode falar",
	"diga",
	"diz",
	"tudo",
	"tudo bem",
	"tudo bom",
	"beleza",
	"joia",
	"estou ouvindo",
	"to ouvindo",
	"na escuta",
	"pode dizer",
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
					highConfidenceVM = cfg.VoicemailPhrases
				}
				if len(cfg.HumanGreetings) > 0 {
					quickHumanGreetings = cfg.HumanGreetings
				}
				break
			}
		}
	}
}

// normalizeText remove acentos, pontuação e múltiplos espaços
func normalizeText(s string) string {
	if s == "" {
		return ""
	}
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	res, _, err := transform.String(t, s)
	if err != nil {
		res = s
	}

	var sb strings.Builder
	for _, r := range strings.ToLower(res) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
		} else {
			sb.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(sb.String()), " ")
}

// levenshtein calcula a distância de edição entre duas palavras
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	dp := make([][]int, la+1)
	for i := range dp {
		dp[i] = make([]int, lb+1)
		dp[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		dp[0][j] = j
	}

	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			dp[i][j] = int(math.Min(
				float64(dp[i-1][j]+1),
				math.Min(float64(dp[i][j-1]+1), float64(dp[i-1][j-1]+cost)),
			))
		}
	}
	return dp[la][lb]
}

// fuzzyContainsPhrase busca a frase permitindo variações acústicas do Vosk (ex: "caixa postar" -> "caixa postal")
func fuzzyContainsPhrase(text, targetPhrase string, maxDistance int) bool {
	textWords := strings.Fields(text)
	targetWords := strings.Fields(targetPhrase)

	if len(targetWords) == 0 || len(textWords) < len(targetWords) {
		return false
	}

	targetJoined := strings.Join(targetWords, " ")
	windowSize := len(targetWords)

	for i := 0; i <= len(textWords)-windowSize; i++ {
		window := strings.Join(textWords[i:i+windowSize], " ")
		if levenshtein(window, targetJoined) <= maxDistance {
			return true
		}
	}
	return false
}

// ClassifyCall AMD de alto rendimento combinando fonética, vocabulário e métricas de cadência
func ClassifyCall(metrics CallMetrics) (status, reason string) {
	norm := normalizeText(metrics.FullText)
	words := strings.Fields(norm)
	wordCount := len(words)

	// 1. CHECAGEM INEQUÍVOCA DE CAIXA POSTAL (com tolerância fonética a ruído de 8kHz)
	for _, phrase := range highConfidenceVM {
		normPhrase := normalizeText(phrase)
		// Tolerância: 1 erro para frases de até 2 palavras, 2 erros para frases mais longas
		maxTolerance := 1
		if len(strings.Fields(normPhrase)) > 2 {
			maxTolerance = 2
		}

		if fuzzyContainsPhrase(norm, normPhrase, maxTolerance) {
			return "MACHINE", "VOICEMAIL_MATCH_" + strings.ToUpper(strings.ReplaceAll(normPhrase, " ", "_"))
		}
	}

	// 2. DETECÇÃO DE SAUDAÇÃO HUMANA TÍPICA (Rápida + Silêncio posterior)
	// Humano fala "Alô?", "Oi, bom dia" e cala a boca esperando a resposta.
	for _, greeting := range quickHumanGreetings {
		normGreeting := normalizeText(greeting)
		paddedText := " " + norm + " "
		paddedGreeting := " " + normGreeting + " "

		if strings.Contains(paddedText, paddedGreeting) {
			// Se disse uma saudação e falou menos de 6 palavras no total, é humano com certeza
			if wordCount <= 5 {
				return "HUMAN", "HUMAN_GREETING_BRIEF"
			}
		}
	}

	// 3. ANÁLISE DE MONÓLOGO CONTÍNUO (Comum em URA e secretária não mapeada)
	// Se falou sem parar por mais de 4 segundos e mais de 10 palavras sem pausa relevante, é máquina
	if metrics.SpeechDurationSec >= 4.0 && wordCount >= 10 && metrics.SilenceAfterSec < 1.0 {
		return "MACHINE", "CONTINUOUS_MONOLOGUE_DETECTED"
	}

	// 4. CLIENTE EM SILÊNCIO OU RESPOSTA CURTA NATURAL
	// Atendeu e ficou em silêncio ou falou poucas palavras ("pois não", "quem é", "quem fala")
	if wordCount <= 3 {
		return "HUMAN", "HUMAN_NATURAL_PAUSE"
	}

	// 5. REGRA DE SEGURANÇA: Se houver dúvida razoável, transfere para o operador
	return "HUMAN", "FALLBACK_ASSUMED_HUMAN"
}

// ClassifyOutcome provê compatibilidade com chamadas legado do classificador
func ClassifyOutcome(fullText string, vmPhrases, greetings []string) (status, cause string) {
	return ClassifyCall(CallMetrics{
		FullText: fullText,
	})
}
