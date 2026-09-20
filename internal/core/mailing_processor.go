package core

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

type MailingProcessor struct {
	storage      ports.StoragePort
	repo         ports.LeadRepository
	cache        ports.CachePort
	audioWordMgr *AudioWordManager
}

func NewMailingProcessor(storage ports.StoragePort, repo ports.LeadRepository, cache ports.CachePort) *MailingProcessor {
	return &MailingProcessor{
		storage: storage,
		repo:    repo,
		cache:   cache,
	}
}

// SetAudioWordManager injeta o gerenciador de áudios para pré-renderização de nomes.
func (mp *MailingProcessor) SetAudioWordManager(mgr *AudioWordManager) {
	mp.audioWordMgr = mgr
}

// ProcessZipRefill executa o streaming do ZIP, valida as primeiras 30 linhas e persiste os leads.
func (mp *MailingProcessor) ProcessZipRefill(ctx context.Context, tenantID, campaignID, fileURI string) (*domain.RefillResponse, error) {
	stream, err := mp.storage.DownloadFileStream(ctx, fileURI)
	if err != nil {
		return nil, domain.NewErrBadRequest("STORAGE_DOWNLOAD_FAILED", fmt.Sprintf("Falha ao baixar arquivo do storage: %s", err.Error()))
	}
	defer stream.Close()

	// Lê o ZIP em memória (suportando até 50MB em buffer)
	buf, err := io.ReadAll(stream)
	if err != nil {
		return nil, domain.NewErrBadRequest("READ_STREAM_FAILED", "Falha ao ler stream do arquivo")
	}

	zipReader, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		return nil, domain.NewErrBadRequest("INVALID_ZIP_ARCHIVE", "O arquivo fornecido não é um arquivo ZIP válido")
	}

	var csvFile io.ReadCloser
	for _, f := range zipReader.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".csv") {
			csvFile, err = f.Open()
			if err != nil {
				return nil, domain.NewErrBadRequest("ZIP_EXTRACTION_ERROR", "Falha ao extrair arquivo CSV do ZIP")
			}
			break
		}
	}

	if csvFile == nil {
		return nil, domain.NewErrBadRequest("NO_CSV_IN_ZIP", "Nenhum arquivo .csv encontrado no pacote compactado")
	}
	defer csvFile.Close()

	reader := csv.NewReader(csvFile)
	reader.Comma = ','
	reader.FieldsPerRecord = -1

	var leads []*domain.Lead
	var phones []string
	var totalRows, validRows, invalidRows int64
	first30Failed := 0
	uniqueNames := make(map[string]string)
	var csvHeaders []string

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			invalidRows++
			if totalRows < 30 {
				first30Failed++
			}
			totalRows++
			continue
		}

		totalRows++
		if len(record) < 4 {
			invalidRows++
			if totalRows <= 30 {
				first30Failed++
			}
			continue
		}

		cpf := strings.TrimSpace(record[0])
		phone := strings.TrimSpace(record[1])
		rowCampID := strings.TrimSpace(record[2])
		rowTenantID := strings.TrimSpace(record[3])

		// Captura cabeçalho canônico CSV (ex: cpf,telefone,campanha_id,tenant_id,name,first_name,campo1,campo2...)
		if totalRows == 1 && strings.EqualFold(cpf, "cpf") && (strings.EqualFold(phone, "telefone") || strings.EqualFold(phone, "phone")) {
			csvHeaders = make([]string, len(record))
			for idx, h := range record {
				cleanH := strings.TrimSpace(h)
				cleanH = strings.Trim(cleanH, `"'`)
				csvHeaders[idx] = cleanH
			}
			totalRows = 0
			continue
		}

		// Validação canônica
		if len(phone) < 8 || rowCampID != campaignID || rowTenantID != tenantID {
			invalidRows++
			if totalRows <= 30 {
				first30Failed++
			}
			// Regra de Corrupção Total: se as primeiras 30 linhas falharem seguidas, aborta imediatamente
			if totalRows == 30 && first30Failed == 30 {
				return nil, domain.NewErrBadRequest("CORRUPTED_FILE",
					"Arquivo corrompido: as 30 primeiras linhas são inválidas ou violam o layout 'cpf,telefone,campanha_id,tenant_id'")
			}
			continue
		}

		name := ""
		firstNameRaw := ""
		if len(record) >= 5 {
			name = strings.TrimSpace(record[4])
		}

		// Captura colunas dinâmicas ou reconstitui first_name com vírgula desprotegida
		var custom map[string]string
		if len(csvHeaders) > 6 {
			// Cabeçalho define colunas dinâmicas além de first_name
			if len(record) >= 6 {
				firstNameRaw = strings.TrimSpace(record[5])
			}
			if len(record) > 6 {
				custom = make(map[string]string)
				for i := 6; i < len(record); i++ {
					key := fmt.Sprintf("col_%d", i+1)
					if i < len(csvHeaders) && csvHeaders[i] != "" {
						key = csvHeaders[i]
					}
					val := strings.TrimSpace(record[i])
					custom[key] = val
				}
			}
		} else if len(csvHeaders) == 0 && len(record) > 6 {
			// Sem cabeçalho, colunas adicionais viram col_7, col_8...
			if len(record) >= 6 {
				firstNameRaw = strings.TrimSpace(record[5])
			}
			custom = make(map[string]string)
			for i := 6; i < len(record); i++ {
				key := fmt.Sprintf("col_%d", i+1)
				val := strings.TrimSpace(record[i])
				custom[key] = val
			}
		} else {
			// Cabeçalho tem até 6 colunas: colunas extras a partir da 6ª são partes de first_name com vírgula
			if len(record) == 6 {
				firstNameRaw = strings.TrimSpace(record[5])
			} else if len(record) > 6 {
				firstNameRaw = strings.TrimSpace(strings.Join(record[5:], ","))
			}
		}
		firstNameRaw = strings.ReplaceAll(firstNameRaw, "\\,", ",")

		pronunciationText := firstNameRaw
		if pronunciationText == "" && name != "" {
			parts := strings.Fields(name)
			if len(parts) > 0 {
				pronunciationText = parts[0]
			}
		}

		normFirstName := domain.Slugify(pronunciationText)
		if normFirstName != "" && pronunciationText != "" {
			if mp.audioWordMgr == nil || !mp.audioWordMgr.HasNameAudio(normFirstName) {
				if _, exists := uniqueNames[normFirstName]; !exists {
					uniqueNames[normFirstName] = pronunciationText
				}
			}
		}

		validRows++
		leads = append(leads, &domain.Lead{
			CampaignID:    campaignID,
			TenantID:      tenantID,
			CPF:           cpf,
			Phone:         phone,
			Name:          name,
			FirstName:     normFirstName,
			WorkWord:      "",
			Custom:        custom,
			Status:        domain.LeadStatusNew,
			AttemptsCount: 0,
		})
		if name != "" || normFirstName != "" || len(custom) > 0 {
			item := domain.LeadQueueItem{
				Phone:     phone,
				CPF:       cpf,
				Name:      name,
				FirstName: normFirstName,
				WorkWord:  "",
				Custom:    custom,
			}
			b, _ := json.Marshal(item)
			phones = append(phones, string(b))
		} else {
			phones = append(phones, phone)
		}
	}

	if validRows == 0 {
		return nil, domain.NewErrBadRequest("EMPTY_VALID_LEADS", "Nenhum lead válido foi extraído do arquivo")
	}

	// Síntese em lote de nomes únicos ausentes O(1)
	if mp.audioWordMgr != nil && len(uniqueNames) > 0 {
		_, _ = mp.audioWordMgr.BatchProcessNames(ctx, uniqueNames)
	}

	// Persiste em batch no Postgres
	_, err = mp.repo.BatchInsert(ctx, leads)
	if err != nil {
		return nil, domain.NewErrInternal(fmt.Sprintf("Falha ao persistir leads no banco de dados: %s", err.Error()))
	}

	// Enfileira no Redis para consumo imediato do discador preditivo
	_ = mp.cache.PushLeads(ctx, campaignID, phones)
	qSize, _ := mp.cache.GetQueueLength(ctx, campaignID)

	return &domain.RefillResponse{
		CampaignID:  campaignID,
		TenantID:    tenantID,
		TotalRows:   totalRows,
		ValidRows:   validRows,
		InvalidRows: invalidRows,
		Status:      "INGESTED",
		QueueSize:   qSize,
	}, nil
}
