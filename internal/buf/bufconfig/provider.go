package bufconfig

import (
	"context"
	"io/ioutil"

	"github.com/bufbuild/buf/internal/buf/bufcheck/bufbreaking"
	"github.com/bufbuild/buf/internal/buf/bufcheck/buflint"
	"github.com/bufbuild/buf/internal/pkg/storage"
	"github.com/bufbuild/buf/internal/pkg/util/utilencoding"
	"github.com/bufbuild/buf/internal/pkg/util/utillog"
	"go.uber.org/multierr"
	"go.uber.org/zap"
)

type provider struct {
	logger                 *zap.Logger
	externalConfigModifier func(*ExternalConfig) error
}

func newProvider(logger *zap.Logger, options ...ProviderOption) *provider {
	provider := &provider{
		logger: logger.Named("config"),
	}
	for _, option := range options {
		option(provider)
	}
	return provider
}

func (p *provider) GetConfigForReadBucket(ctx context.Context, readBucket storage.ReadBucket) (_ *Config, retErr error) {
	defer utillog.Defer(p.logger, "get_config_for_bucket")()

	externalConfig := &ExternalConfig{}
	readObject, err := readBucket.Get(ctx, ConfigFilePath)
	if err != nil {
		if storage.IsNotExist(err) {
			return p.newConfig(externalConfig)
		}
		return nil, err
	}
	defer func() {
		retErr = multierr.Append(retErr, readObject.Close())
	}()
	data, err := ioutil.ReadAll(readObject)
	if err != nil {
		return nil, err
	}
	if err := utilencoding.UnmarshalYAMLStrict(data, externalConfig); err != nil {
		return nil, err
	}
	return p.newConfig(externalConfig)
}

func (p *provider) GetConfigForData(data []byte) (*Config, error) {
	defer utillog.Defer(p.logger, "get_config_for_data")()

	externalConfig := &ExternalConfig{}
	if err := utilencoding.UnmarshalJSONOrYAMLStrict(data, externalConfig); err != nil {
		return nil, err
	}
	return p.newConfig(externalConfig)
}

func (p *provider) newConfig(externalConfig *ExternalConfig) (*Config, error) {
	if p.externalConfigModifier != nil {
		if err := p.externalConfigModifier(externalConfig); err != nil {
			return nil, err
		}
	}
	breakingConfig, err := bufbreaking.ConfigBuilder{
		Use:                           externalConfig.Breaking.Use,
		Except:                        externalConfig.Breaking.Except,
		IgnoreRootPaths:               externalConfig.Breaking.Ignore,
		IgnoreIDOrCategoryToRootPaths: externalConfig.Breaking.IgnoreOnly,
	}.NewConfig()
	if err != nil {
		return nil, err
	}
	lintConfig, err := buflint.ConfigBuilder{
		Use:                                  externalConfig.Lint.Use,
		Except:                               externalConfig.Lint.Except,
		IgnoreRootPaths:                      externalConfig.Lint.Ignore,
		IgnoreIDOrCategoryToRootPaths:        externalConfig.Lint.IgnoreOnly,
		EnumZeroValueSuffix:                  externalConfig.Lint.EnumZeroValueSuffix,
		RPCAllowSameRequestResponse:          externalConfig.Lint.RPCAllowSameRequestResponse,
		RPCAllowGoogleProtobufEmptyRequests:  externalConfig.Lint.RPCAllowGoogleProtobufEmptyRequests,
		RPCAllowGoogleProtobufEmptyResponses: externalConfig.Lint.RPCAllowGoogleProtobufEmptyResponses,
		ServiceSuffix:                        externalConfig.Lint.ServiceSuffix,
	}.NewConfig()
	if err != nil {
		return nil, err
	}
	return &Config{
		Build:    externalConfig.Build,
		Breaking: breakingConfig,
		Lint:     lintConfig,
	}, nil
}
