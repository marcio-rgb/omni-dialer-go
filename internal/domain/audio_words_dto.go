package domain

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var slugCleanRegex = regexp.MustCompile(`[^a-z0-9_]+`)

// Slugify remove acentos, pontuação e normaliza strings para nomes de arquivo seguros
func Slugify(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	clean, _, _ := transform.String(t, strings.TrimSpace(s))
	clean = strings.ToLower(clean)
	clean = strings.ReplaceAll(clean, " ", "_")
	clean = strings.ReplaceAll(clean, "-", "_")
	clean = slugCleanRegex.ReplaceAllString(clean, "")
	clean = strings.Trim(clean, "_")
	for strings.Contains(clean, "__") {
		clean = strings.ReplaceAll(clean, "__", "_")
	}
	return clean
}

// WordItemDTO representa uma palavra individual a ser pré-renderizada
type WordItemDTO struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// CustomPhrasesDTO contém as frases-base padrão configuráveis da campanha
type CustomPhrasesDTO struct {
	Saudacao   string `json:"saudacao"`
	FaloCom    string `json:"falo_com"`
	Momentinho string `json:"momentinho"`
}

// UpsertAudioWordsRequest representa o contrato de entrada para pré-renderizar palavras
type UpsertAudioWordsRequest struct {
	CustomPhrases  *CustomPhrasesDTO `json:"custom_phrases,omitempty"`
	PausaMs        int               `json:"pausa_ms"`
	Pausas         []int             `json:"pausas,omitempty"`
	CustomPausas   *CustomPausasDTO  `json:"custom_pausas,omitempty"`
	Names          []WordItemDTO     `json:"names"`
	WorkWords      []WordItemDTO     `json:"work_words"`
	ForceOverwrite bool              `json:"force_overwrite"`
}

// UnmarshalJSON permite parsing flexível de payloads aninhados ou com campos planos (ex: first_name, work_words como string)
func (r *UpsertAudioWordsRequest) UnmarshalJSON(data []byte) error {
	type rawRequest struct {
		CustomPhrases  *CustomPhrasesDTO `json:"custom_phrases"`
		Saudacao       string            `json:"saudacao"`
		FaloCom        string            `json:"falo_com"`
		Momentinho     string            `json:"momentinho"`
		FirstName      string            `json:"first_name"`
		Name           string            `json:"name"`
		PausaMs        json.RawMessage   `json:"pausa_ms"`
		Pausas         json.RawMessage   `json:"pausas"`
		CustomPausas   *CustomPausasDTO  `json:"custom_pausas"`
		Names          json.RawMessage   `json:"names"`
		WorkWords      json.RawMessage   `json:"work_words"`
		ForceOverwrite *bool             `json:"force_overwrite"`
	}

	var raw rawRequest
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// 1. PausaMs (padrão 0ms: pausas agora são tratadas diretamente no texto)
	r.PausaMs = 0
	if len(raw.PausaMs) > 0 {
		var pInt int
		if err := json.Unmarshal(raw.PausaMs, &pInt); err == nil {
			r.PausaMs = pInt
		} else {
			var pStr string
			if err := json.Unmarshal(raw.PausaMs, &pStr); err == nil {
				cleanStr := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(pStr), "ms"))
				if v, err := strconv.Atoi(cleanStr); err == nil {
					r.PausaMs = v
				}
			}
		}
	}

	// 2. Pausas customizadas
	if raw.CustomPausas != nil {
		r.CustomPausas = raw.CustomPausas
	}
	if len(raw.Pausas) > 0 {
		var pList []int
		if err := json.Unmarshal(raw.Pausas, &pList); err == nil {
			r.Pausas = pList
		} else {
			var pObj CustomPausasDTO
			if err := json.Unmarshal(raw.Pausas, &pObj); err == nil {
				r.CustomPausas = &pObj
			}
		}
	}

	// 3. Custom Phrases
	if raw.CustomPhrases != nil {
		r.CustomPhrases = raw.CustomPhrases
	}
	if raw.Saudacao != "" || raw.FaloCom != "" || raw.Momentinho != "" {
		if r.CustomPhrases == nil {
			r.CustomPhrases = &CustomPhrasesDTO{}
		}
		if raw.Saudacao != "" {
			r.CustomPhrases.Saudacao = raw.Saudacao
		}
		if raw.FaloCom != "" {
			r.CustomPhrases.FaloCom = raw.FaloCom
		}
		if raw.Momentinho != "" {
			r.CustomPhrases.Momentinho = raw.Momentinho
		}
	}

	// 3. Names & FirstName
	r.Names = make([]WordItemDTO, 0)
	if raw.FirstName != "" {
		r.Names = append(r.Names, WordItemDTO{
			Key:  Slugify(raw.FirstName),
			Text: raw.FirstName,
		})
	} else if raw.Name != "" {
		r.Names = append(r.Names, WordItemDTO{
			Key:  Slugify(raw.Name),
			Text: raw.Name,
		})
	}
	if len(raw.Names) > 0 {
		var items []WordItemDTO
		if err := json.Unmarshal(raw.Names, &items); err == nil {
			r.Names = append(r.Names, items...)
		} else {
			var strList []string
			if err := json.Unmarshal(raw.Names, &strList); err == nil {
				for _, s := range strList {
					r.Names = append(r.Names, WordItemDTO{Key: Slugify(s), Text: s})
				}
			}
		}
	}

	// 4. WorkWords
	r.WorkWords = make([]WordItemDTO, 0)
	if len(raw.WorkWords) > 0 {
		var items []WordItemDTO
		if err := json.Unmarshal(raw.WorkWords, &items); err == nil {
			r.WorkWords = append(r.WorkWords, items...)
		} else {
			var singleStr string
			if err := json.Unmarshal(raw.WorkWords, &singleStr); err == nil {
				r.WorkWords = append(r.WorkWords, WordItemDTO{Key: Slugify(singleStr), Text: singleStr})
			} else {
				var strList []string
				if err := json.Unmarshal(raw.WorkWords, &strList); err == nil {
					for _, s := range strList {
						r.WorkWords = append(r.WorkWords, WordItemDTO{Key: Slugify(s), Text: s})
					}
				}
			}
		}
	}

	// 5. ForceOverwrite
	if raw.ForceOverwrite != nil {
		r.ForceOverwrite = *raw.ForceOverwrite
	} else {
		r.ForceOverwrite = true
	}

	return nil
}

// Validate valida a integridade do DTO de entrada
func (r *UpsertAudioWordsRequest) Validate() error {
	if r.PausaMs < 0 || r.PausaMs > 5000 {
		return errors.New("pausa_ms deve estar entre 0 e 5000 ms")
	}
	if len(r.Names) == 0 && len(r.WorkWords) == 0 && r.CustomPhrases == nil {
		return errors.New("pelo menos uma palavra (nome, convenio ou frase-base) deve ser informada")
	}
	for i := range r.Names {
		if strings.TrimSpace(r.Names[i].Key) == "" && strings.TrimSpace(r.Names[i].Text) != "" {
			r.Names[i].Key = Slugify(r.Names[i].Text)
		}
		if strings.TrimSpace(r.Names[i].Key) == "" || strings.TrimSpace(r.Names[i].Text) == "" {
			return errors.New("cada item de 'names' deve conter 'key' e 'text' nao vazios")
		}
	}
	for i := range r.WorkWords {
		if strings.TrimSpace(r.WorkWords[i].Key) == "" && strings.TrimSpace(r.WorkWords[i].Text) != "" {
			r.WorkWords[i].Key = Slugify(r.WorkWords[i].Text)
		}
		if strings.TrimSpace(r.WorkWords[i].Key) == "" || strings.TrimSpace(r.WorkWords[i].Text) == "" {
			return errors.New("cada item de 'work_words' deve conter 'key' e 'text' nao vazios")
		}
	}
	return nil
}

// UpsertAudioWordsResponse representa o resultado detalhado do processamento
type UpsertAudioWordsResponse struct {
	Success        bool            `json:"success"`
	TotalRequested int             `json:"total_requested"`
	CreatedCount   int             `json:"created_count"`
	CachedCount    int             `json:"cached_count"`
	ElapsedMs      int64           `json:"elapsed_ms"`
	Files          AudioFilesMapDTO `json:"files"`
}

// AudioFilesMapDTO lista os arquivos processados por categoria
type AudioFilesMapDTO struct {
	Base      []string `json:"base"`
	Names     []string `json:"names"`
	WorkWords []string `json:"work_words"`
}

// CustomPausasDTO permite especificar pausas personalizadas em milissegundos para cada transição
type CustomPausasDTO struct {
	SaudacaoConvenioMs int `json:"saudacao_convenio_ms"`
	ConvenioFaloComMs  int `json:"convenio_falo_com_ms"`
	FaloComNomeMs      int `json:"falo_com_nome_ms"`
	NomeMomentinhoMs   int `json:"nome_momentinho_ms"`
}

// AudioPreviewRequest define os parâmetros para concatenação e teste do áudio completo
type AudioPreviewRequest struct {
	Name              string           `json:"name"`
	WorkWord          string           `json:"work_word"`
	PausaMs           int              `json:"pausa_ms"`
	Pausas            []int            `json:"pausas"`
	CustomPausas      *CustomPausasDTO `json:"custom_pausas"`
	IncludeMomentinho bool             `json:"include_momentinho"`
}

// GetPausesSlice devolve a lista ordenada de pausas em ms para cada transição
func (r *AudioPreviewRequest) GetPausesSlice() []int {
	if r.CustomPausas != nil {
		return []int{
			r.CustomPausas.SaudacaoConvenioMs,
			r.CustomPausas.ConvenioFaloComMs,
			r.CustomPausas.FaloComNomeMs,
			r.CustomPausas.NomeMomentinhoMs,
		}
	}
	if len(r.Pausas) > 0 {
		return r.Pausas
	}
	return []int{r.PausaMs, r.PausaMs, r.PausaMs, r.PausaMs}
}

// UnmarshalJSON permite aceitar 'first_name', 'work_words', 'pausas', 'custom_pausas', 'pausa_ms' e 'momentinho'
func (r *AudioPreviewRequest) UnmarshalJSON(data []byte) error {
	type rawPreview struct {
		Name              string           `json:"name"`
		FirstName         string           `json:"first_name"`
		WorkWord          string           `json:"work_word"`
		WorkWords         string           `json:"work_words"`
		PausaMs           json.RawMessage  `json:"pausa_ms"`
		Pausas            json.RawMessage  `json:"pausas"`
		CustomPausas      *CustomPausasDTO `json:"custom_pausas"`
		IncludeMomentinho *bool            `json:"include_momentinho"`
		Momentinho        json.RawMessage  `json:"momentinho"`
	}

	var raw rawPreview
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.Name = raw.Name
	if r.Name == "" {
		r.Name = raw.FirstName
	}

	r.WorkWord = raw.WorkWord
	if r.WorkWord == "" {
		r.WorkWord = raw.WorkWords
	}

	r.PausaMs = 0
	if len(raw.PausaMs) > 0 {
		var pInt int
		if err := json.Unmarshal(raw.PausaMs, &pInt); err == nil {
			r.PausaMs = pInt
		} else {
			var pStr string
			if err := json.Unmarshal(raw.PausaMs, &pStr); err == nil {
				cleanStr := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(pStr), "ms"))
				if v, err := strconv.Atoi(cleanStr); err == nil {
					r.PausaMs = v
				}
			}
		}
	}

	if raw.CustomPausas != nil {
		r.CustomPausas = raw.CustomPausas
	}
	if len(raw.Pausas) > 0 {
		var pList []int
		if err := json.Unmarshal(raw.Pausas, &pList); err == nil {
			r.Pausas = pList
		} else {
			var pObj CustomPausasDTO
			if err := json.Unmarshal(raw.Pausas, &pObj); err == nil {
				r.CustomPausas = &pObj
			}
		}
	}

	if raw.IncludeMomentinho != nil {
		r.IncludeMomentinho = *raw.IncludeMomentinho
	} else if len(raw.Momentinho) > 0 {
		var b bool
		if err := json.Unmarshal(raw.Momentinho, &b); err == nil {
			r.IncludeMomentinho = b
		} else {
			var s string
			if err := json.Unmarshal(raw.Momentinho, &s); err == nil && s != "" {
				r.IncludeMomentinho = true
			}
		}
	}

	return nil
}

// Validate valida os parâmetros da requisição de preview
func (r *AudioPreviewRequest) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("o parametro 'name' (ou 'first_name') e obrigatorio")
	}
	if strings.TrimSpace(r.WorkWord) == "" {
		return errors.New("o parametro 'work_word' (ou 'work_words') e obrigatorio")
	}
	if r.PausaMs < 0 || r.PausaMs > 2000 {
		r.PausaMs = 60 // Padrão seguro
	}
	return nil
}
