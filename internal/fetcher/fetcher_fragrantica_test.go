package fetcher

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFragranticaFetcher_Handle(t *testing.T) {
	l := logger.NewTestLogger()

	mockClient := NewMockHTTPClient(t)

	htmlContent, err := os.ReadFile("testdata/fragrantica_success.html")
	require.NoError(t, err, "Failed to read test HTML file")

	mockClient.EXPECT().
		Do(mock.AnythingOfType("*http.Request")).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(htmlContent)),
			Header: http.Header{
				"Content-Type": []string{"text/html; charset=utf-8"},
			},
		}, nil)

	fetcher := NewFragranticaFetcher(l, mockClient)

	request, err := NewFragranticaRequest(
		"https://www.fragrantica.com/perfume/Zara/Marshmallow-Addiction-Eau-de-Parfum-81569.html",
		5,
	)
	require.NoError(t, err, "Failed to create request")

	response, err := fetcher.Handle(request)
	t.Logf("TEST %v", l.GetEntries())
	require.NoError(t, err, "Handle should not return error")
	assert.False(t, response.IsError, "Response should not be an error")
	assert.Len(t, response.Content, 1, "Should have exactly one content item")
	assert.Equal(t, ContentTypeText, response.Content[0].Type, "Content type should be text")

	expectedText := `Title: Marshmallow Addiction Eau de Parfum Zara perfume - a fragrance for women 2023
Description: Marshmallow Addiction Eau de Parfum by Zara is a Oriental Vanilla fragrance for women. Marshmallow Addiction Eau de Parfum was launched in 2023. Top notes are Fruits and Sunflower; middle note is Vanilla; base note is Musk.
Rating: 3.82/5 (votes: 458)
Accords: fruity, sweet, vanilla, musky, powdery
Notes: Top: Fruits, Sunflower; Middle: Vanilla; Base: Musk
Similar: Sunrise On The Red Sand Dunes Zara, Sunrise on the Red Sand Dunes Intense Zara, Ebony Wood Zara, Elegantly Tokyo Zara, Golden Decade Zara
Reviews:
- Rianna22 (12/10/25 13:36): A beautiful scent! Smells like juicy sweet apricots!!♥️
And as it is, it's a linear scent, do not expect big things from it.
I regret to not buying it, even tho it's technically barely lasts more than an hour...😅
Maybe I try to tame a 30ml bottle later, to enhance longevity with some body oil.
- miss mills (06/18/25 11:24): I took my 11 year old niece perfume shopping at Zara for her birthday and this is the one she fell in love with. It's a sweet fruity summery delight. Not getting the marshmallow but suspect the vanilla and musk is what makes this smell gourmand rather than straight up fruit juice. Good longevity on my skin - 3-4 hrs and not too overpowering. Pretty packaging too.
- scythess (05/12/25 17:29): Extremely fruity, the name isn't accurate at all! No marshmallow to be found. It is sweet and pleasant but doesn't last long. Summery and almost tropical. Like fruity gummy candy
- shawnaceline (04/03/25 14:32): Smell good but it dissappeard within an hour ,so won't be buying it again
- Dop@mine (03/29/25 10:09): I received this as a free sample with a decant order. It's fine, but definitely not for me. I probably would have liked it when I was younger though. It's simple, fruity and clean. Like everyone else, I'm mystified by the choice of name. I wonder why they chose to call it that?
`

	assert.Equal(t, expectedText, response.Content[0].Text, "Response text should match expected")
}

func TestFragranticaFetcher_getComRequest(t *testing.T) {
	l := logger.NewTestLogger()
	mockClient := NewMockHTTPClient(t)
	fetcher := NewFragranticaFetcher(l, mockClient)

	tests := []struct {
		name        string
		inputURL    string
		expectedURL string
		expectError bool
	}{
		{
			name:        "valid URL with www",
			inputURL:    "https://www.fragrantica.com/perfume/Chanel/Coco-Mademoiselle-117.html",
			expectedURL: "https://www.fragrantica.com/perfume/Chanel/Coco-Mademoiselle-117.html",
			expectError: false,
		},
		{
			name:        "valid URL without www",
			inputURL:    "https://fragrantica.ru/perfume/Chanel/Coco-Mademoiselle-117.html",
			expectedURL: "https://www.fragrantica.com/perfume/Chanel/Coco-Mademoiselle-117.html",
			expectError: false,
		},
		{
			name:        "invalid URL",
			inputURL:    "https://example.com/invalid",
			expectedURL: "",
			expectError: true,
		},
		{
			name:        "URL with different domain",
			inputURL:    "https://fragrantica.ru/perfume/Chanel/Coco-Mademoiselle-117.html",
			expectedURL: "https://www.fragrantica.com/perfume/Chanel/Coco-Mademoiselle-117.html",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := NewRequestPayload(tt.inputURL, nil, nil)
			require.NoError(t, err, "Failed to create request")

			comRequest, err := fetcher.getComRequest(request)

			if tt.expectError {
				assert.Error(t, err, "Expected error")
				assert.Nil(t, comRequest, "Request should be nil on error")
			} else {
				assert.NoError(t, err, "Should not return error")
				assert.NotNil(t, comRequest, "Request should not be nil")
				assert.Equal(t, tt.expectedURL, comRequest.URL(), "URL should be normalized")
			}
		})
	}
}
