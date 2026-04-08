package config

type Config struct {
	DBPath              string
	SessionsDir         string
	MCPPort             int
	MetricsPort         int
	OllamaURL           string
	OllamaModel         string
	RRFWeightFTS        float64
	RRFWeightVec        float64
	LogLevel            string
	CrossServiceMode    string // "none" | "lite" | "strict"
	MemoryRodURL        string
	CrossServiceTimeout string
}

func Default() Config {
	return Config{
		DBPath:              "/opt/plan/data/plan.db",
		SessionsDir:         "/opt/plan/sessions",
		MCPPort:             9101,
		MetricsPort:         9111,
		OllamaURL:           "http://localhost:11434",
		OllamaModel:         "nomic-embed-text",
		RRFWeightFTS:        0.4,
		RRFWeightVec:        0.6,
		LogLevel:            "info",
		CrossServiceMode:    "lite",
		MemoryRodURL:        "http://localhost:9100/mcp",
		CrossServiceTimeout: "500ms",
	}
}
