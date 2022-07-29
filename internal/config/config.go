package config

type TelegramAPI struct {
	Token    string
	Endpoint string
}

type HTTPProxy struct {
	AdvertisedEndpoint string `mapstructure:"advertised_endpoint"`
	Listen             string
	Allow              []string
}

type GRPCProxy struct {
	Listen string
}

type Client struct {
	Name  string
	Token string
}

type Config struct {
	TelegramAPI TelegramAPI `mapstructure:"telegram"`
	HTTPProxy   HTTPProxy   `mapstructure:"http"`
	GRPCProxy   GRPCProxy   `mapstructure:"grpc"`
	Clients     []Client
}
