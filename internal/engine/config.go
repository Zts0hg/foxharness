package engine

type Config struct {
	EnableThinking bool
	MaxTurns       int
}

func DefaultConfig() Config {
	return Config{
		EnableThinking: false,
		MaxTurns:       20,
	}
}
