package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config centraliza todas as variáveis de ambiente necessárias para a operação do Dialer-Go.
type Config struct {
	AppPort             int
	DatabaseURL         string
	RedisAddr           string
	RedisPassword       string
	RedisDB             int
	AsteriskAMIHost     string
	AsteriskAMIPort     int
	AsteriskAMIUser     string
	AsteriskAMIPass     string
	MinIOEndpoint       string
	MinIOAccessKey      string
	MinIOSecretKey      string
	MinIOBucket         string
	MinIOUseSSL         bool
	InitialWhitelistIPs []string
	PacingInterval      time.Duration
	MaxGlobalChannels   int
	HumanReserveQuota   int
	MinChannelsPerAgent int
	WebhookURL          string
	RecordingsBaseURL   string
	OmniChatWebhookURL  string
}

// Load lê as variáveis de ambiente e aplica valores default para o ambiente de produção.
func Load() (*Config, error) {
	appPort, _ := strconv.Atoi(getEnv("PORT", "8080"))
	redisDB, _ := strconv.Atoi(getEnv("REDIS_DB", "0"))
	amiPort, _ := strconv.Atoi(getEnv("ASTERISK_AMI_PORT", "5038"))
	useSSL, _ := strconv.ParseBool(getEnv("MINIO_USE_SSL", "false"))
	maxChannels, _ := strconv.Atoi(getEnv("MAX_GLOBAL_CHANNELS", "120"))
	humanQuota, _ := strconv.Atoi(getEnv("HUMAN_RESERVED_QUOTA", "10"))
	minChannelsPerAgent, _ := strconv.Atoi(getEnv("MIN_CHANNELS_PER_AGENT", "7"))
	if minChannelsPerAgent <= 0 {
		minChannelsPerAgent = 7
	}

	whitelistRaw := getEnv("INITIAL_WHITELIST_IPS", "127.0.0.1,::1")
	whitelist := strings.Split(whitelistRaw, ",")
	for i := range whitelist {
		whitelist[i] = strings.TrimSpace(whitelist[i])
	}

	webhookURL := getEnv("WEBHOOK_URL", "")
	if webhookURL == "" {
		webhookURL = getEnv("OMNICHAT_WEBHOOK_URL", "")
	}

	cfg := &Config{
		AppPort:             appPort,
		DatabaseURL:         getEnv("DATABASE_URL", "postgres://dialer_user:dialer_secret@localhost:5432/dialer_db?sslmode=disable"),
		RedisAddr:           getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:       getEnv("REDIS_PASSWORD", ""),
		RedisDB:             redisDB,
		AsteriskAMIHost:     getEnv("ASTERISK_AMI_HOST", "127.0.0.1"),
		AsteriskAMIPort:     amiPort,
		AsteriskAMIUser:     getEnv("ASTERISK_AMI_USER", "dialer_admin"),
		AsteriskAMIPass:     getEnv("ASTERISK_AMI_PASS", "dialer_secret_ami"),
		MinIOEndpoint:       getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:      getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey:      getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinIOBucket:         getEnv("MINIO_BUCKET", "mailings"),
		MinIOUseSSL:         useSSL,
		InitialWhitelistIPs: whitelist,
		PacingInterval:      time.Duration(1) * time.Second,
		MaxGlobalChannels:   maxChannels,
		HumanReserveQuota:   humanQuota,
		MinChannelsPerAgent: minChannelsPerAgent,
		WebhookURL:          webhookURL,
		RecordingsBaseURL:   getEnv("RECORDINGS_BASE_URL", ""),
		OmniChatWebhookURL:  webhookURL,
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL é obrigatória")
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
