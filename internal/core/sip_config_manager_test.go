package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"dialer-go/internal/domain"
)

type mockSIPConfigRepo struct {
	files map[string]*domain.SIPConfigFile
}

func newMockSIPConfigRepo() *mockSIPConfigRepo {
	return &mockSIPConfigRepo{
		files: make(map[string]*domain.SIPConfigFile),
	}
}

func (m *mockSIPConfigRepo) GetFile(ctx context.Context, filename string) (*domain.SIPConfigFile, error) {
	if f, ok := m.files[filename]; ok {
		return f, nil
	}
	return nil, nil
}

func (m *mockSIPConfigRepo) ListFiles(ctx context.Context) ([]domain.SIPConfigFile, error) {
	var list []domain.SIPConfigFile
	for _, f := range m.files {
		list = append(list, *f)
	}
	return list, nil
}

func (m *mockSIPConfigRepo) SaveFile(ctx context.Context, file *domain.SIPConfigFile) error {
	m.files[file.File] = file
	return nil
}

func (m *mockSIPConfigRepo) DeleteFile(ctx context.Context, filename string) error {
	delete(m.files, filename)
	return nil
}

func TestSIPConfigManager_Lifecycle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asterisk-test-conf")
	if err != nil {
		t.Fatalf("falha ao criar temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	repo := newMockSIPConfigRepo()
	mgr := NewSIPConfigManager(repo, tmpDir)

	ctx := context.Background()

	// 1. SaveFile
	err = mgr.SaveFile(ctx, &domain.SIPConfigFile{
		File: "pjsip.conf",
		Data: "[global]\ntype=global\n",
	})
	if err != nil {
		t.Fatalf("SaveFile falhou: %v", err)
	}

	// 2. GetFile
	f, err := mgr.GetFile(ctx, "pjsip.conf")
	if err != nil || f == nil {
		t.Fatalf("GetFile falhou: %v", err)
	}
	if f.Data != "[global]\ntype=global\n" {
		t.Errorf("conteudo divergente: %s", f.Data)
	}

	// 3. ApplyConfigs
	mockAMI := &mockAMIForReload{connected: true}
	resp, err := mgr.ApplyConfigs(ctx, []string{"pjsip.conf"}, mockAMI)
	if err != nil || !resp.Success {
		t.Fatalf("ApplyConfigs falhou: %v, resp: %+v", err, resp)
	}

	// Verifica se arquivo fisico foi gravado
	writtenPath := filepath.Join(tmpDir, "pjsip.conf")
	data, err := os.ReadFile(writtenPath)
	if err != nil {
		t.Fatalf("arquivo fisico nao foi gravado em %s: %v", writtenPath, err)
	}
	if string(data) != "[global]\ntype=global\n" {
		t.Errorf("conteudo em disco incorreto: %s", string(data))
	}
}
