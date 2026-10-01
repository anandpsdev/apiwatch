package config

import "time"

type Config struct {
	File      FileConfig
	Capture   CaptureConfig
	Retention RetentionConfig
	Server    ServerConfig
	OnError   func(error)
}

type FileConfig struct {
	Directory          string
	LockExternalAccess bool
	FilePermissions    uint32
}

type CaptureConfig struct {
	CaptureRequestBody   bool
	CaptureResponseBody  bool
	MaxRequestBodySize   int64
	MaxResponseBodySize  int64
	RedactHeaders        []string
	RedactFields         []string
	ExcludePaths         []string
	SlowRequestThreshold time.Duration
}

type RetentionConfig struct {
	MaxAge       time.Duration
	MaxFileSize  int64
	MaxTotalSize int64
	MaxFiles     int
}

type ServerConfig struct {
	Path string
}

func Default() Config {
	return Config{
		File: FileConfig{
			Directory:          "./logs/apiwatch",
			LockExternalAccess: true,
			FilePermissions:    0600,
		},
		Capture: CaptureConfig{
			CaptureRequestBody:   true,
			CaptureResponseBody:  true,
			MaxRequestBodySize:   64 * 1024,
			MaxResponseBodySize:  64 * 1024,
			SlowRequestThreshold: 1 * time.Second,
			ExcludePaths: []string{
				"/apiwatch",
				"/apiwatch/",
			},
			RedactHeaders: []string{
				"Authorization",
				"Cookie",
				"Set-Cookie",
				"X-API-Key",
				"Proxy-Authorization",
			},
			RedactFields: []string{
				"password",
				"passwd",
				"token",
				"access_token",
				"refresh_token",
				"api_key",
				"apikey",
				"secret",
				"client_secret",
				"private_key",
				"cvv",
				"card_number",
			},
		},
		Retention: RetentionConfig{
			MaxAge:       7 * 24 * time.Hour,
			MaxFileSize:  100 * 1024 * 1024,
			MaxTotalSize: 500 * 1024 * 1024,
			MaxFiles:     30,
		},
		Server: ServerConfig{
			Path: "/apiwatch",
		},
	}
}

func (c Config) Normalize() Config {
	d := Default()
	if c.File.Directory == "" {
		c.File.Directory = d.File.Directory
	}
	if c.File.FilePermissions == 0 {
		c.File.FilePermissions = d.File.FilePermissions
	}
	if c.Capture.MaxRequestBodySize <= 0 {
		c.Capture.MaxRequestBodySize = d.Capture.MaxRequestBodySize
	}
	if c.Capture.MaxResponseBodySize <= 0 {
		c.Capture.MaxResponseBodySize = d.Capture.MaxResponseBodySize
	}
	if c.Capture.SlowRequestThreshold <= 0 {
		c.Capture.SlowRequestThreshold = d.Capture.SlowRequestThreshold
	}
	if len(c.Capture.RedactHeaders) == 0 {
		c.Capture.RedactHeaders = d.Capture.RedactHeaders
	}
	if len(c.Capture.RedactFields) == 0 {
		c.Capture.RedactFields = d.Capture.RedactFields
	}
	if len(c.Capture.ExcludePaths) == 0 {
		c.Capture.ExcludePaths = d.Capture.ExcludePaths
	}
	if c.Retention.MaxAge <= 0 {
		c.Retention.MaxAge = d.Retention.MaxAge
	}
	if c.Retention.MaxFileSize <= 0 {
		c.Retention.MaxFileSize = d.Retention.MaxFileSize
	}
	if c.Retention.MaxTotalSize <= 0 {
		c.Retention.MaxTotalSize = d.Retention.MaxTotalSize
	}
	if c.Retention.MaxFiles <= 0 {
		c.Retention.MaxFiles = d.Retention.MaxFiles
	}
	if c.Server.Path == "" {
		c.Server.Path = d.Server.Path
	}
	return c
}
