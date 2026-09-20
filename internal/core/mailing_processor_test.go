package core

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

type mockStorageForMailing struct {
	ports.StoragePort
	data []byte
}

func (m *mockStorageForMailing) DownloadFileStream(ctx context.Context, fileURI string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(m.data)), nil
}

type mockLeadRepoForMailing struct {
	ports.LeadRepository
	inserted []*domain.Lead
}

func (m *mockLeadRepoForMailing) BatchInsert(ctx context.Context, leads []*domain.Lead) (int64, error) {
	m.inserted = append(m.inserted, leads...)
	return int64(len(leads)), nil
}

func createTestZip(csvContent string) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	w, _ := zw.Create("leads.csv")
	_, _ = w.Write([]byte(csvContent))
	_ = zw.Close()
	return buf.Bytes()
}

func TestMailingProcessor_CommasInFirstName(t *testing.T) {
	ctx := context.Background()

	// 1. Linha com aspas RFC 4180 contendo vírgula
	// 2. Linha sem aspas com vírgula no campo first_name (reconstituição automática)
	// 3. Linha com vírgula escapada (\,)
	csvData := strings.Join([]string{
		"cpf,telefone,campanha_id,tenant_id,name,first_name",
		`11122233344,11988887777,camp101,tenant1,MARCIO NASCIMENTO,"; Márcio, tudo bem? ;"`,
		`55566677788,11977776666,camp101,tenant1,JOAO SILVA,; João, beleza? ;`,
		`99900011122,11966665555,camp101,tenant1,PEDRO SOUZA,; Pedro\, como vai? ;`,
	}, "\n")

	zipBytes := createTestZip(csvData)

	storage := &mockStorageForMailing{data: zipBytes}
	repo := &mockLeadRepoForMailing{}
	cache := &mockCachePredictive{}

	processor := NewMailingProcessor(storage, repo, cache)

	resp, err := processor.ProcessZipRefill(ctx, "tenant1", "camp101", "s3://test.zip")
	if err != nil {
		t.Fatalf("ProcessZipRefill falhou: %v", err)
	}

	if resp.ValidRows != 3 {
		t.Fatalf("esperava 3 linhas válidas, obteve %d", resp.ValidRows)
	}

	if len(repo.inserted) != 3 {
		t.Fatalf("esperava 3 leads persistidos, obteve %d", len(repo.inserted))
	}

	// 1. Quoted RFC 4180
	if repo.inserted[0].Name != "MARCIO NASCIMENTO" {
		t.Errorf("lead 0 Name incorreto: %s", repo.inserted[0].Name)
	}
	if repo.inserted[0].FirstName != "marcio_tudo_bem" {
		t.Errorf("lead 0 FirstName slug incorreto: '%s' (esperava 'marcio_tudo_bem')", repo.inserted[0].FirstName)
	}

	// 2. Unquoted CSV com vírgula no first_name
	if repo.inserted[1].Name != "JOAO SILVA" {
		t.Errorf("lead 1 Name incorreto: %s", repo.inserted[1].Name)
	}
	if repo.inserted[1].FirstName != "joao_beleza" {
		t.Errorf("lead 1 FirstName slug incorreto: '%s' (esperava 'joao_beleza')", repo.inserted[1].FirstName)
	}

	// 3. Escaped comma (\,)
	if repo.inserted[2].Name != "PEDRO SOUZA" {
		t.Errorf("lead 2 Name incorreto: %s", repo.inserted[2].Name)
	}
	if repo.inserted[2].FirstName != "pedro_como_vai" {
		t.Errorf("lead 2 FirstName slug incorreto: '%s' (esperava 'pedro_como_vai')", repo.inserted[2].FirstName)
	}
}

func TestMailingProcessor_DynamicColumns(t *testing.T) {
	ctx := context.Background()

	// CSV com colunas dinâmicas: renda, banco, contrato, valor_saldo
	csvData := strings.Join([]string{
		"cpf,telefone,campanha_id,tenant_id,name,first_name,renda,banco,contrato,valor_saldo",
		`12345678901,11988887777,camp101,tenant1,Carlos Eduardo,Carlos,5000,033,CTR-9988,1250.75`,
		`98765432100,21977776666,camp101,tenant1,Fernanda Lima,Fernanda,7500,237,CTR-1122,3400.00`,
	}, "\n")

	zipBytes := createTestZip(csvData)

	storage := &mockStorageForMailing{data: zipBytes}
	repo := &mockLeadRepoForMailing{}
	cache := &mockCachePredictive{}

	processor := NewMailingProcessor(storage, repo, cache)

	resp, err := processor.ProcessZipRefill(ctx, "tenant1", "camp101", "s3://test.zip")
	if err != nil {
		t.Fatalf("ProcessZipRefill falhou: %v", err)
	}

	if resp.ValidRows != 2 {
		t.Fatalf("esperava 2 linhas válidas, obteve %d", resp.ValidRows)
	}

	if len(repo.inserted) != 2 {
		t.Fatalf("esperava 2 leads persistidos, obteve %d", len(repo.inserted))
	}

	lead0 := repo.inserted[0]
	if lead0.Name != "Carlos Eduardo" {
		t.Errorf("lead 0 Name incorreto: %s", lead0.Name)
	}
	if lead0.FirstName != "carlos" {
		t.Errorf("lead 0 FirstName incorreto: %s", lead0.FirstName)
	}
	if lead0.Custom == nil {
		t.Fatalf("lead 0 Custom é nil")
	}
	if lead0.Custom["renda"] != "5000" {
		t.Errorf("lead 0 Custom[renda] incorreto: %s (esperava 5000)", lead0.Custom["renda"])
	}
	if lead0.Custom["banco"] != "033" {
		t.Errorf("lead 0 Custom[banco] incorreto: %s (esperava 033)", lead0.Custom["banco"])
	}
	if lead0.Custom["contrato"] != "CTR-9988" {
		t.Errorf("lead 0 Custom[contrato] incorreto: %s (esperava CTR-9988)", lead0.Custom["contrato"])
	}
	if lead0.Custom["valor_saldo"] != "1250.75" {
		t.Errorf("lead 0 Custom[valor_saldo] incorreto: %s (esperava 1250.75)", lead0.Custom["valor_saldo"])
	}

	// Verifica se foi colocado no Redis queue com Custom preenchido
	if len(cache.queue) != 2 {
		t.Fatalf("esperava 2 itens na fila do cache, obteve %d", len(cache.queue))
	}

	var qItem domain.LeadQueueItem
	if err := json.Unmarshal([]byte(cache.queue[0]), &qItem); err != nil {
		t.Fatalf("falha ao deserializar LeadQueueItem: %v", err)
	}
	if qItem.Custom["renda"] != "5000" || qItem.Custom["banco"] != "033" {
		t.Errorf("LeadQueueItem Custom incorreto: %v", qItem.Custom)
	}
}

