package config

import (
	"strings"

	"places-fetcher/exception"

	"github.com/spf13/viper"
)

type MainConfig struct {
	Places   PlacesConfig
	Overpass OverpassConfig
	Sheet    SheetConfig
	Outlet   OutletConfig
	Promo    PromoConfig
	WebPage  WebPageConfig
}

type PlacesConfig struct {
	Provider string `mapstructure:"Provider"` // registered adapter name, e.g. "overpass"
	BBox     string `mapstructure:"BBox"`     // south,west,north,east
}

type OverpassConfig struct {
	Endpoints        []string `mapstructure:"Endpoints"`
	UserAgent        string   `mapstructure:"UserAgent"`
	TimeoutSeconds   int      `mapstructure:"TimeoutSeconds"`
	Retries          int      `mapstructure:"Retries"`
	RetryWaitSeconds int      `mapstructure:"RetryWaitSeconds"`
}

type SheetConfig struct {
	URL            string `mapstructure:"URL"`
	Token          string `mapstructure:"Token"`
	TimeoutSeconds int    `mapstructure:"TimeoutSeconds"`
}

type OutletConfig struct {
	MergeRadiusM float64 `mapstructure:"MergeRadiusM"`
}

// Settings of the promo job (go run ./app/promos); each source adapter reads its own section.
type PromoConfig struct {
	UserAgent           string `mapstructure:"UserAgent"`
	FetchTimeoutSeconds int    `mapstructure:"FetchTimeoutSeconds"` // per source, all its pages
	MaxErrors           int    `mapstructure:"MaxErrors"`           // failed runs in a row before status error
}

// Settings of the web_page source adapter (promo/source/webpage).
type WebPageConfig struct {
	TimeoutSeconds int `mapstructure:"TimeoutSeconds"` // per request
	Retries        int `mapstructure:"Retries"`        // after a 429, 5xx or network error
	HostDelayMs    int `mapstructure:"HostDelayMs"`    // gap between requests to the same host
	MaxNewItems    int `mapstructure:"MaxNewItems"`    // detail pages opened per source per run
}

// LoadConfig reads config.yaml; any key can be overridden by env, e.g. Sheet.Token → SHEET_TOKEN.
func LoadConfig(path string) (config MainConfig) {
	viper.AddConfigPath(path)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	exception.PanicIfNeeded(err)

	err = viper.Unmarshal(&config)
	exception.PanicIfNeeded(err)

	return config
}
