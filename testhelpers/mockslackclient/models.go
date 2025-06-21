package mockslackclient

import (
	"encoding/json"
	"fmt"
	"strings"
)

type BlocksWrapper struct {
	Blocks []Block `json:"blocks"`
}

func (b BlocksWrapper) GetHeaderTexts() (string, string) {
	var firstHeader string
	var secondHeader string
	for blockIdx, block := range b.Blocks {
		if block.Type == "header" && block.Text != nil {
			if blockIdx == 0 {
				firstHeader = block.Text.Text
			} else {
				secondHeader = block.Text.Text
			}
		}
	}
	return firstHeader, secondHeader
}

func (b BlocksWrapper) GetPRItemTexts() []string {
	var prTexts []string
	for _, block := range b.Blocks {
		if block.Type == "rich_text" && block.Elements != nil {
			var richTextLists []RichTextList // we're expecting an array of one
			err := json.Unmarshal(block.Elements, &richTextLists)
			if err != nil {
				panic(fmt.Sprintf("Unexpected rich_text list array type: %v", err))
			}
			if len(richTextLists) != 1 {
				panic(fmt.Sprintf("Expected exactly one rich_text list, got %d", len(richTextLists)))
			}
			listItemsElements := richTextLists[0].Elements
			for _, section := range listItemsElements {
				prText := ""
				for _, element := range section.Elements {
					if element.Text != "" {
						prText += element.Text
					}
				}
				prTexts = append(prTexts, prText)
			}

		}
	}
	return prTexts
}

func (b BlocksWrapper) ContainsPRTitle(title string) bool {
	for _, item := range b.GetPRItemTexts() {
		if strings.Contains(item, title) {
			return true
		}
	}
	return false
}

type Block struct {
	Type     string          `json:"type"`
	Text     *TextObject     `json:"text,omitempty"`
	BlockID  string          `json:"block_id,omitempty"`
	Elements json.RawMessage `json:"elements,omitempty"` // We'll unmarshal this based on Type
}

type TextObject struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Emoji bool   `json:"emoji,omitempty"`
}

type RichTextList struct {
	Type     string            `json:"type"`
	Elements []RichTextSection `json:"elements"`
	Style    string            `json:"style"`
	Indent   int               `json:"indent"`
	Border   int               `json:"border"`
	Offset   int               `json:"offset"`
}

type RichTextSection struct {
	Type     string    `json:"type"`
	Elements []Element `json:"elements"`
}

type Element struct {
	Type   string        `json:"type"`
	Text   string        `json:"text,omitempty"`
	URL    string        `json:"url,omitempty"`
	UserID string        `json:"user_id,omitempty"`
	Style  *ElementStyle `json:"style,omitempty"`
}

type ElementStyle struct {
	Bold bool `json:"bold,omitempty"`
}

func ParseBlocks(data []byte) (BlocksWrapper, error) {
	var blocks []Block
	err := json.Unmarshal(data, &blocks)
	return BlocksWrapper{Blocks: blocks}, err
}
