package registry

import (
	"fmt"

	"github.com/goccy/go-json"

	"github.com/forta-protocol/forta-core-go/contracts"
	"github.com/forta-protocol/forta-core-go/utils"
)

var SaveAgent = "SaveAgent"
var DisableAgent = "DisableAgent"
var EnableAgent = "EnableAgent"

type AgentMessage struct {
	Message
	AgentID string `json:"agentId"`
	TxHash  string `json:"txHash"`
}

type AgentSaveMessage struct {
	AgentMessage
	Enabled  bool    `json:"enabled"`
	Name     string  `json:"name"`
	ChainIDs []int64 `json:"chainIds"`
	Metadata string  `json:"metadata"`
	Owner    string  `json:"owner"`
}

func ParseAgentSave(msg string) (*AgentSaveMessage, error) {
	var save AgentSaveMessage
	err := json.Unmarshal([]byte(msg), &save)
	if err != nil {
		return nil, err
	}
	if save.Action != SaveAgent {
		return nil, fmt.Errorf("invalid action for AgentSave: %s", save.Action)
	}
	return &save, nil
}

func ParseAgentMessage(msg string) (*AgentMessage, error) {
	var m AgentMessage
	err := json.Unmarshal([]byte(msg), &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func NewAgentMessage(evt *contracts.AgentRegistryAgentEnabled) *AgentMessage {
	agentID := utils.Hex(evt.AgentId)
	evtName := DisableAgent
	if evt.Enabled {
		evtName = EnableAgent
	}
	return &AgentMessage{
		Message: Message{
			Action: evtName,
		},
		AgentID: agentID,
		TxHash:  evt.Raw.TxHash.Hex(),
	}
}

func NewAgentSaveMessage(evt *contracts.AgentRegistryAgentUpdated) *AgentSaveMessage {
	agentID := utils.Hex(evt.AgentId)
	return &AgentSaveMessage{
		AgentMessage: AgentMessage{
			AgentID: agentID,
			Message: Message{
				Action: SaveAgent,
			},
			TxHash: evt.Raw.TxHash.Hex(),
		},
		Enabled:  true,
		Name:     evt.Metadata,
		ChainIDs: utils.IntArray(evt.ChainIds),
		Metadata: evt.Metadata,
		Owner:    evt.By.Hex(),
	}
}
