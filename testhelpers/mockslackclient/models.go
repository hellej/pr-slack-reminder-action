package mockslackclient

import (
	"encoding/json"
	"fmt"
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
		if block.IsHeading() {
			var richTextSections []RichTextSection
			err := json.Unmarshal(block.Elements, &richTextSections)
			if err != nil {
				panic(fmt.Sprintf("Unexpected rich_text section array type: %v", err))
			}
			if len(richTextSections) > 0 {
				headingText := ""
				for _, element := range richTextSections[0].Elements {
					if element.Text != "" {
						headingText += element.Text
					}
				}
				currentHeading = headingText
			}
		}
		var prList PRList
		if currentHeading != "" && block.IsPRItem() {
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
						text := element.Text
						if element.Style != nil && element.Style.Strike {
							text = "~" + text + "~"
						}
						prText += text
					}
					if element.UserID != "" {
						prText += element.UserID
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

func (b BlocksWrapper) GetAllPRItemTexts() []string {
	var allTexts []string
	for _, item := range b.GetPRLists() {
		allTexts = append(allTexts, item.PRListItems...)
	}
	return allTexts
}

func (b BlocksWrapper) SomePRItemContainsText(searchText string) bool {
	for _, text := range b.GetAllPRItemTexts() {
		if strings.Contains(text, searchText) {
			return true
		}
	}
	return false
}

func (b BlocksWrapper) SomePRItemTextIsEqualTo(searchText string) bool {
	for _, text := range b.GetAllPRItemTexts() {
		if text == searchText {
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
	return len(b.GetAllPRItemTexts())
}

type Block struct {
	Type     string          `json:"type"`
	Text     *TextObject     `json:"text,omitempty"`
	BlockID  string          `json:"block_id,omitempty"`
	Elements json.RawMessage `json:"elements,omitempty"` // We'll unmarshal this based on Type
}

func (b Block) IsHeading() bool {
	return b.Type == "rich_text" && (strings.HasPrefix(b.BlockID, "pr_list_heading"))
}

func (b Block) IsPRItem() bool {
	return strings.HasPrefix(b.BlockID, "open_prs")
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
	Bold   bool `json:"bold,omitempty"`
	Strike bool `json:"strike,omitempty"`
}

func ParseBlocks(data []byte) (BlocksWrapper, error) {
	var blocks []Block
	err := json.Unmarshal(data, &blocks)
	return BlocksWrapper{Blocks: blocks}, err
}
