package config

type Config struct {
	SessionFile string
	OutputDir   string
	S3Config    *S3Config
}

type S3Config struct {
	AccessKey  string
	SecretKey  string
	BucketName string
	ServiceUrl string
	BucketUrl  string
}

func NewConfig() *Config {
	return &Config{
		SessionFile: "whatsapp.session",
		OutputDir:   "media",
		S3Config: &S3Config{
			AccessKey:  "AKIAR7HWXVBBO3CDTQNF",
			SecretKey:  "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",
			BucketName: "ligchat-whatsapp",
			ServiceUrl: "https://s3.amazonaws.com",
			BucketUrl:  "https://ligchat-whatsapp.s3.amazonaws.com",
		},
	}
}
