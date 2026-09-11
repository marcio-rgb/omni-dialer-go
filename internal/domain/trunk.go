package domain

import (
	"fmt"
	"time"
)

type RegistrationMode string
const (
	RegistrationModeRegister RegistrationMode = "REGISTER"
	RegistrationModeIPBased  RegistrationMode = "IP_BASED"
)

type Transport string
const (
	TransportUDP Transport = "UDP"
	TransportTCP Transport = "TCP"
	TransportTLS Transport = "TLS"
)

type Direction string
const (
	DirectionBidirectional Direction = "BIDIRECTIONAL"
	DirectionOutbound      Direction = "OUTBOUND"
	DirectionInbound       Direction = "INBOUND"
)

type DTMFMode string
const (
	DTMFModeRFC4733 DTMFMode = "RFC4733"
	DTMFModeRFC2833 DTMFMode = "RFC2833"
	DTMFModeInBand  DTMFMode = "INBAND"
	DTMFModeInfo    DTMFMode = "INFO"
	DTMFModeAuto    DTMFMode = "AUTO"
)

type NATMode string
const (
	NATModeForceRPort NATMode = "FORCE_RPORT"
	NATModeYes        NATMode = "YES"
	NATModeNo         NATMode = "NO"
	NATModeComedia    NATMode = "COMEDIA"
)

// Trunk representa a entidade relacional de tronco telefônico SIP/PJSIP.
type Trunk struct {
	ID               string           `json:"id"`
	TenantID         string           `json:"tenant_id"`
	Name             string           `json:"name"`
	Direction        Direction        `json:"direction"`
	RegistrationMode RegistrationMode `json:"registration_mode"`
	Host             string           `json:"host"`
	Port             int              `json:"port"`
	OutboundProxy    *string          `json:"outbound_proxy,omitempty"`
	TechPrefix       *string          `json:"tech_prefix,omitempty"`
	AuthUsername     *string          `json:"auth_username,omitempty"`
	AuthPassword     *string          `json:"auth_password,omitempty"`
	AuthRealm        *string          `json:"auth_realm,omitempty"`
	FromUser         *string          `json:"from_user,omitempty"`
	FromDomain       *string          `json:"from_domain,omitempty"`
	UserAgent        *string          `json:"user_agent,omitempty"`
	Transport        Transport        `json:"transport"`
	NATMode          NATMode          `json:"nat_mode"`
	DirectMedia      bool             `json:"direct_media"`
	Codecs           []string         `json:"codecs"`
	DTMFMode         DTMFMode         `json:"dtmf_mode"`
	RTPTimeout       int              `json:"rtp_timeout"`
	QualifyFrequency int              `json:"qualify_frequency"`
	QualifyTimeout   float64          `json:"qualify_timeout"`
	MaxChannels      int              `json:"max_channels"`
	IsEnabled        bool             `json:"is_enabled"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

// DialString gera a string de canal Asterisk considerando ou não o tech_prefix.
func (t *Trunk) DialString(destinationPhone string) string {
	phone := destinationPhone
	if t.TechPrefix != nil && *t.TechPrefix != "" {
		phone = fmt.Sprintf("%s%s", *t.TechPrefix, destinationPhone)
	}
	if t.Host != "" {
		return fmt.Sprintf("PJSIP/%s/sip:%s@%s", t.ID, phone, t.Host)
	}
	return fmt.Sprintf("PJSIP/%s@%s", phone, t.ID)
}

// TrunkHealth representa o estado volátil e a telemetria em tempo real do tronco.
type TrunkHealth struct {
	Status                string    `json:"status"` // ONLINE, UNREACHABLE, REGISTERED, REJECTED
	LatencyMS             float64   `json:"latency_ms"`
	ActiveChannels        int       `json:"active_channels"`
	MaxChannels           int       `json:"max_channels"`
	AvailableChannels     int       `json:"available_channels"`
	UtilizationPercentage float64   `json:"utilization_percentage"`
	IsSaturated           bool      `json:"is_saturated"`
	LastQualifyAt         time.Time `json:"last_qualify_at"`
	ErrorMessage          *string   `json:"error_message,omitempty"`
}

// TrunkWithHealth agrega o tronco com sua telemetria para listagem HTTP.
type TrunkWithHealth struct {
	Trunk
	Health TrunkHealth `json:"health"`
}

// DTOs de Entrada e Saída
type CreateTrunkDTO struct {
	ID               string           `json:"id"`
	TenantID         string           `json:"tenant_id"`
	Name             string           `json:"name"`
	Direction        Direction        `json:"direction"`
	RegistrationMode RegistrationMode `json:"registration_mode"`
	Host             string           `json:"host"`
	Port             int              `json:"port"`
	OutboundProxy    *string          `json:"outbound_proxy,omitempty"`
	TechPrefix       *string          `json:"tech_prefix,omitempty"`
	AuthUsername     *string          `json:"auth_username,omitempty"`
	AuthPassword     *string          `json:"auth_password,omitempty"`
	AuthRealm        *string          `json:"auth_realm,omitempty"`
	FromUser         *string          `json:"from_user,omitempty"`
	FromDomain       *string          `json:"from_domain,omitempty"`
	UserAgent        *string          `json:"user_agent,omitempty"`
	Transport        Transport        `json:"transport"`
	NATMode          NATMode          `json:"nat_mode"`
	Codecs           []string         `json:"codecs"`
	DTMFMode         DTMFMode         `json:"dtmf_mode"`
	QualifyFrequency int              `json:"qualify_frequency"`
	QualifyTimeout   float64          `json:"qualify_timeout"`
	MaxChannels      int              `json:"max_channels"`
	IsEnabled        bool             `json:"is_enabled"`
}

type UpdateTrunkDTO struct {
	Name             *string           `json:"name,omitempty"`
	Direction        *Direction        `json:"direction,omitempty"`
	RegistrationMode *RegistrationMode `json:"registration_mode,omitempty"`
	Host             *string           `json:"host,omitempty"`
	Port             *int              `json:"port,omitempty"`
	OutboundProxy    *string           `json:"outbound_proxy,omitempty"`
	TechPrefix       *string           `json:"tech_prefix,omitempty"`
	AuthUsername     *string           `json:"auth_username,omitempty"`
	AuthPassword     *string           `json:"auth_password,omitempty"`
	AuthRealm        *string           `json:"auth_realm,omitempty"`
	FromUser         *string           `json:"from_user,omitempty"`
	FromDomain       *string           `json:"from_domain,omitempty"`
	UserAgent        *string           `json:"user_agent,omitempty"`
	Transport        *Transport        `json:"transport,omitempty"`
	NATMode          *NATMode          `json:"nat_mode,omitempty"`
	Codecs           []string          `json:"codecs,omitempty"`
	DTMFMode         *DTMFMode         `json:"dtmf_mode,omitempty"`
	QualifyFrequency *int              `json:"qualify_frequency,omitempty"`
	QualifyTimeout   *float64          `json:"qualify_timeout,omitempty"`
	MaxChannels      *int              `json:"max_channels,omitempty"`
	IsEnabled        *bool             `json:"is_enabled,omitempty"`
}

type EnumOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type TrunkEnumerationsDTO struct {
	RegistrationModes []EnumOption `json:"registration_modes"`
	Transports        []EnumOption `json:"transports"`
	Directions        []EnumOption `json:"directions"`
	SupportedCodecs   []EnumOption `json:"supported_codecs"`
	DTMFModes         []EnumOption `json:"dtmf_modes"`
	NATModes          []EnumOption `json:"nat_modes"`
}
