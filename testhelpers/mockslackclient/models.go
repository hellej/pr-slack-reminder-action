package mockslackclient

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
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

// Each bullet list becomes one PRList under the heading above it: the repository's sub-heading
// when grouped by repository, the section's header block when not.
func (b BlocksWrapper) GetPRLists() []PRList {
	prLists := []PRList{}
	currentHeading := ""
	for _, block := range b.Blocks {
		if block.Type == "header" && block.Text != nil {
			currentHeading = block.Text.Text
			continue
		}
		if block.isRepositorySubHeading() {
			currentHeading = block.repositorySubHeadingLinkText()
			continue
		}
		if !block.holdsPRRows() {
			continue
		}
		for _, list := range block.richTextElements() {
			prLists = append(prLists, PRList{
				Heading:     currentHeading,
				PRListItems: utilities.Map(list.listItems(), listItemText),
			})
		}
	}
	return prLists
}

func listItemText(listItem RichTextElement) string {
	return concatenatedText(listItem.textRuns())
}

func concatenatedText(elements []Element) string {
	text := ""
	for _, element := range elements {
		text += element.Text + element.UserID
	}
	return text
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
	BlockID  string          `json:"block_id,omitempty"`
	Text     *Element        `json:"text,omitempty"`     // Set on a header block only
	Elements json.RawMessage `json:"elements,omitempty"` // We'll unmarshal this based on Type
}

func (b Block) holdsPRRows() bool {
	return b.Type == "rich_text" && strings.HasPrefix(b.BlockID, "section_")
}

func (b Block) isRepositorySubHeading() bool {
	return b.Type == "context" && strings.HasPrefix(b.BlockID, "section_")
}

// The sub-heading is one mrkdwn element reading "*<url|name:>*".
func (b Block) repositorySubHeadingLinkText() string {
	var elements []Element
	if err := json.Unmarshal(b.Elements, &elements); err != nil {
		panic(fmt.Sprintf("Unexpected context element array type: %v", err))
	}
	_, linkTextAndEnd, _ := strings.Cut(elements[0].Text, "|")
	linkText, _, _ := strings.Cut(linkTextAndEnd, ">")
	return linkText
}

func (b Block) richTextElements() []RichTextElement {
	var elements []RichTextElement
	if err := json.Unmarshal(b.Elements, &elements); err != nil {
		panic(fmt.Sprintf("Unexpected rich_text element array type: %v", err))
	}
	return elements
}

// Both a rich_text_section and a rich_text_list carry an "elements" array, holding text runs
// for the section and list item sections for the list.
type RichTextElement struct {
	Type     string          `json:"type"`
	Elements json.RawMessage `json:"elements"`
}

func (e RichTextElement) textRuns() []Element {
	var runs []Element
	if err := json.Unmarshal(e.Elements, &runs); err != nil {
		panic(fmt.Sprintf("Unexpected rich_text_section element array type: %v", err))
	}
	return runs
}

func (e RichTextElement) listItems() []RichTextElement {
	var items []RichTextElement
	if err := json.Unmarshal(e.Elements, &items); err != nil {
		panic(fmt.Sprintf("Unexpected rich_text_list element array type: %v", err))
	}
	return items
}

type Element struct {
	Text   string `json:"text,omitempty"`
	UserID string `json:"user_id,omitempty"`
}

func parseBlocks(data []byte) (BlocksWrapper, error) {
	var blocks []Block
	err := json.Unmarshal(data, &blocks)
	return BlocksWrapper{Blocks: blocks}, err
}
