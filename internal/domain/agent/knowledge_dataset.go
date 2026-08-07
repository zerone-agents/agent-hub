package agent

import "time"

// AgentKnowledgeDataset records the datasets bound to an agent for knowledge
// retrieval. The dataset metadata (name/description) is kept in multirag;
// control-panel only stores the dataset IDs and uses them for authorization
// and to assemble the deployer request.
type AgentKnowledgeDataset struct {
	AgentID   uint64    `gorm:"primaryKey;index:idx_agent_id"`
	DatasetID string    `gorm:"primaryKey;type:varchar(64)"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (AgentKnowledgeDataset) TableName() string {
	return "agent_knowledge_datasets"
}
