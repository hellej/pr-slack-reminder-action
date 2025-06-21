package mockslackclient

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// A data model for Blocks that were sent to Slack API.
// Provides a helper function for tests for checking if a specific PR title is present in the blocks.
type BlocksWrapper struct {
	Blocks []Block `json:"blocks"`
}

type PRList struct {
	Heading     string
	PRListItems []string
}

func (b BlocksWrapper) GetPRLists() []PRList {
	prLists := []PRList{}

	currentHeading := ""
	for _, block := range b.Blocks {
		if block.Type == "header" && block.Text != nil {
			currentHeading = block.Text.Text
		}
		var prList PRList
		if currentHeading != "" && block.Type == "rich_text" && block.Elements != nil {
			prList.Heading = currentHeading
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
				prList.PRListItems = append(prList.PRListItems, prText)
			}

		}
		if prList.Heading != "" || len(prList.PRListItems) > 0 {
			prLists = append(prLists, prList)
		}
	}
	return prLists
}

func (b BlocksWrapper) ContainsPRTitle(title string) bool {
	for _, item := range b.GetPRLists() {
		if slices.ContainsFunc(item.PRListItems, func(value string) bool {
			return strings.Contains(value, title)
		}) {
			return true
		}
	}
	return false
}

func (b BlocksWrapper) ContainsHeading(heading string) bool {
	for _, item := range b.GetPRLists() {
		if item.Heading == heading {
			return true
		}
	}
	return false
}

func (b BlocksWrapper) GetPRCount() int {
	var count int
	for _, item := range b.GetPRLists() {
		count += len(item.PRListItems)
	}
	return count
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
