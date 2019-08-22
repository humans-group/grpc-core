package config

import (
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"
)

type Validatable interface {
	Validate() error
}

type Decodable interface {
	// Fills object by value from data. Must have pointer receiver
	Decode(data interface{}) error
}

var decodableType = reflect.ValueOf((*Decodable)(nil)).Type().Elem()

type Config struct {
	v       *viper.Viper
	lock    sync.RWMutex
	watches map[string]func()
}

type Loader interface {
	MustLoad(key string, to interface{})
	Load(key string, to interface{}) error
	Watch(key string, callback func())
}

var configFile = pflag.StringP("config", "c", "", "configuration file to use")

func Configure() *Config {
	cfg := &Config{v: viper.New()}
	cfg.v.AutomaticEnv()
	cfg.v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// try to load configuration from environment
	if envConfig, ok := os.LookupEnv("CONFIG"); ok {
		cfg.v.SetConfigType("env")
		if err := cfg.v.ReadConfig(strings.NewReader(envConfig)); err != nil {
			panic(errors.Wrap(err, "failed to read configuration from environment: CONFIG"))
		}

		return cfg
	}

	if *configFile != "" {
		cfg.v.SetConfigFile(*configFile)
	} else {
		cfg.v.SetConfigType("yaml")
		cfg.v.SetConfigName("config")

		cfg.v.AddConfigPath(".")
		cfg.v.AddConfigPath("etc")
		cfg.v.AddConfigPath("../etc")
		cfg.v.AddConfigPath("/etc")

	}

	if err := cfg.v.ReadInConfig(); err != nil {
		panic(err)
	}

	cfg.v.WatchConfig()
	cfg.v.OnConfigChange(func(_ fsnotify.Event) {
		//TODO implement diff on current state and call watchers per config key
	})

	return cfg
}

func (c *Config) Watch(key string, callback func()) {
	c.lock.Lock()
	c.watches[key] = callback
	c.lock.Unlock()
}

func (c *Config) MustLoad(key string, to interface{}) {
	if err := c.Load(key, to); err != nil {
		panic(err)
	}
}

func (c *Config) Load(key string, to interface{}) error {
	key = normalizeKey(key)

	raw := c.v.Get(key)
	if raw == nil {
		return errors.Errorf("failed to find key: %q", key)
	}

	decoder, err := newDecoder(to)
	if err != nil {
		return errors.Wrap(err, "failed to create decoder")
	}

	if err := decoder.Decode(raw); err != nil {
		return errors.Wrapf(err, "failed to decode configuration key %s", key)
	}

	return validate(to)
}

func Decode(from, to interface{}) error {
	decoder, err := newDecoder(to)
	if err != nil {
		return errors.Wrap(err, "failed to create config decoder")
	}

	if err := decoder.Decode(from); err != nil {
		return err
	}

	return validate(to)
}

func decodeHook(from reflect.Type, to reflect.Type, data interface{}) (interface{}, error) {
	var (
		typeOfTimeDuration = reflect.TypeOf(time.Duration(0))
		typeOfTime         = reflect.TypeOf(time.Time{})
	)

	if from.Kind() == reflect.String {
		switch to {
		case typeOfTimeDuration:
			converted, err := time.ParseDuration(data.(string))
			if err != nil {
				return nil, errors.Wrapf(err, "error converting %v to %v", data, to)
			}

			return converted, nil
		case typeOfTime:
			tm, err := time.Parse(time.RFC3339, data.(string))
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse %q, time should be specified in RFC3339 format (%s)", data, time.RFC3339)
			}
			return tm, nil
		}
	}

	// here we instantiate pointer to value of type 'to' to check that it implements interface Decodable
	valPtr := reflect.New(to).Elem().Addr()

	if valPtr.Type().Implements(decodableType) {
		val := valPtr.Interface()
		err := val.(Decodable).Decode(data)
		if err != nil {
			return nil, errors.Wrapf(err, "error converting %v to %v", data, to)
		}

		return valPtr.Elem().Interface(), nil
	}

	return data, nil
}

func normalizeKey(key string) string {
	pointIdx := strings.Index(key, ".")
	if pointIdx == -1 {
		return key
	}

	firstSubkey := key[0:pointIdx]
	// lowercase first component to workaround current viper behavior -
	// it does not handle this for keys consisting of multiple parts
	// (but does handle for simple keys consisting of single part)
	return strings.ToLower(firstSubkey) + key[pointIdx:]
}

func validate(v interface{}) error {
	if err := validator.Validate(v); err != nil && err != validator.ErrUnsupported {
		return errors.Wrapf(err, "failed to validate %+v", v)
	}

	if v, ok := v.(Validatable); ok {
		if err := v.Validate(); err != nil {
			return errors.Wrapf(err, "failed to validate %+v", v)
		}
	}

	return nil
}

func newDecoder(to interface{}) (*mapstructure.Decoder, error) {
	return mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook:       decodeHook,
		WeaklyTypedInput: true,
		ErrorUnused:      true,
		TagName:          "key",
		Result:           to,
	})
}
