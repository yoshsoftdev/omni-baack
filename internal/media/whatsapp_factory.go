package media

import (
	"fmt"
	"log/slog"

	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

// NewWhatsAppMediaManagerFromMeta wires WhatsAppMediaManager with a Meta Cloud uploader.
// External packages must use this factory — they cannot obtain or call the Meta uploader directly.
func NewWhatsAppMediaManagerFromMeta(
	assets AssetRepository,
	cache ProviderMediaCacheRepository,
	storage ObjectStore,
	metaClient *metaclient.Client,
	opts ...WhatsAppManagerOption,
) (*WhatsAppMediaManager, error) {
	if metaClient == nil {
		return nil, fmt.Errorf("meta client is required")
	}
	cfg := WhatsAppMediaManagerConfig{
		Assets:      assets,
		Cache:       cache,
		Storage:     storage,
		Uploader:    NewMetaCloudUploader(metaClient),
		Policy:      DefaultWhatsAppMediaCachePolicy(),
		Retry:       DefaultUploadRetryPolicy(),
		RefreshWait: DefaultRefreshWaitPolicy(),
		Metrics:     NoopMetrics{},
		Log:         slog.Default(),
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return NewWhatsAppMediaManager(cfg)
}

// WhatsAppManagerOption configures optional WhatsAppMediaManager behavior.
type WhatsAppManagerOption func(*WhatsAppMediaManagerConfig)

func WithMediaCachePolicy(policy MediaCachePolicy) WhatsAppManagerOption {
	return func(cfg *WhatsAppMediaManagerConfig) {
		if policy != nil {
			cfg.Policy = policy
		}
	}
}

func WithUploadRetryPolicy(policy UploadRetryPolicy) WhatsAppManagerOption {
	return func(cfg *WhatsAppMediaManagerConfig) {
		cfg.Retry = policy
	}
}

func WithRefreshWaitPolicy(policy RefreshWaitPolicy) WhatsAppManagerOption {
	return func(cfg *WhatsAppMediaManagerConfig) {
		cfg.RefreshWait = policy
	}
}

func WithMediaMetrics(metrics Metrics) WhatsAppManagerOption {
	return func(cfg *WhatsAppMediaManagerConfig) {
		if metrics != nil {
			cfg.Metrics = metrics
		}
	}
}

func WithMediaLogger(log *slog.Logger) WhatsAppManagerOption {
	return func(cfg *WhatsAppMediaManagerConfig) {
		if log != nil {
			cfg.Log = log
		}
	}
}
