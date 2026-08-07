package knowledge

import "context"

type KnowledgeEngine interface {
	Health(ctx context.Context) (*HealthStatus, error)
	ListDatasets(ctx context.Context, req DatasetListRequest) (*DatasetListResult, error)
	CreateDataset(ctx context.Context, req DatasetMutationRequest) (*Dataset, error)
	GetDataset(ctx context.Context, id string) (*Dataset, error)
	UpdateDataset(ctx context.Context, id string, req DatasetMutationRequest) (*Dataset, error)
	DeleteDatasets(ctx context.Context, req DeleteRequest) error
	ListDocuments(ctx context.Context, datasetID string, req DocumentListRequest) (*DocumentListResult, error)
	UploadDocuments(ctx context.Context, datasetID string, upload UploadRequest) ([]Document, error)
	DownloadDocument(ctx context.Context, datasetID string, documentID string) (*StreamResult, error)
	GetImage(ctx context.Context, imageID string) (*StreamResult, error)
	UpdateDocument(ctx context.Context, datasetID string, documentID string, req DocumentUpdateRequest) (*Document, error)
	DeleteDocuments(ctx context.Context, datasetID string, req DeleteRequest) error
	ParseDocuments(ctx context.Context, datasetID string, ids []string) error
	StopParsingDocuments(ctx context.Context, datasetID string, ids []string) error
	ListChunks(ctx context.Context, datasetID string, documentID string, req ChunkListRequest) (*ChunkListResult, error)
	CreateChunk(ctx context.Context, datasetID string, documentID string, req ChunkMutationRequest) (*Chunk, error)
	UpdateChunk(ctx context.Context, datasetID string, documentID string, chunkID string, req ChunkMutationRequest) (*Chunk, error)
	DeleteChunks(ctx context.Context, datasetID string, documentID string, req DeleteChunksRequest) error
	SwitchChunks(ctx context.Context, datasetID string, documentID string, ids []string, available bool) error
	Retrieval(ctx context.Context, req RetrievalRequest) (*RetrievalResult, error)
}
