package core

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

type MailingProcessor struct {
	storage ports.StoragePort
	repo    ports.LeadRepository
	cache   ports.CachePort
}

func NewMailingProcessor(storage ports.StoragePort, repo ports.LeadRepository, cache ports.CachePort) *MailingProcessor {
	return &MailingProcessor{
		storage: storage,
		repo:    repo,
		cache:   cache,
	}
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

		// Ignora cabeçalho canônico CSV (ex: cpf,telefone,campanha_id,tenant_id)
		if totalRows == 1 && strings.EqualFold(cpf, "cpf") && (strings.EqualFold(phone, "telefone") || strings.EqualFold(phone, "phone")) {
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

		validRows++
		leads = append(leads, &domain.Lead{
			CampaignID:    campaignID,
			TenantID:      tenantID,
			CPF:           cpf,
			Phone:         phone,
			Status:        domain.LeadStatusNew,
			AttemptsCount: 0,
		})
		phones = append(phones, phone)
	}

	if validRows == 0 {
		return nil, domain.NewErrBadRequest("EMPTY_VALID_LEADS", "Nenhum lead válido foi extraído do arquivo")
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
