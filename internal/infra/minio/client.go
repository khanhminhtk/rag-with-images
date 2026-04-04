package minio

import (
	"rag_imagetotext_texttoimage/internal/util"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Config struct {
	Endpoint        string 
	AccessKey       string 
	SecretKey       string 
	UseSSL          bool
	Region 			string
}

type MinioCleant struct {
	Client *minio.Client
	appLogger util.Logger
}

func NewMinioCleant(appLogger util.Logger, config Config) (*MinioCleant, error) {
	client, err := minio.New(
		config.Endpoint,
		&minio.Options{
			Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
			Secure: config.UseSSL,
			Region: config.Region,
		},
	)
	if err != nil {
		appLogger.Error("internal.infra.minio.client.NewMinioCleant: Don't create minio client due to: ", err)
		return nil, err
	}
	return &MinioCleant{
		Client: client,
		appLogger: appLogger,
	}, nil
}