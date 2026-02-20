package web

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"strings"

	"github.com/vscode-contributor-website/scraper"
)

// Card dimensions (Twitter/OG standard)
const (
	cardWidth  = 1200
	cardHeight = 630
)

// Color palette
var (
	bgColor        = color.RGBA{R: 30, G: 30, B: 46, A: 255}   // Dark background
	primaryColor   = color.RGBA{R: 0, G: 122, B: 204, A: 255}  // VS Code blue
	accentColor    = color.RGBA{R: 31, G: 156, B: 240, A: 255} // Light blue
	textColor      = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	secondaryText  = color.RGBA{R: 180, G: 180, B: 200, A: 255}
	avatarBgColor  = color.RGBA{R: 60, G: 60, B: 80, A: 255}
)

// CardHandler generates a social sharing card image for a contributor
func CardHandler(w http.ResponseWriter, r *http.Request) {
	// Extract username from path: /card/{username}
	path := r.URL.Path
	username := strings.TrimPrefix(path, "/card/")
	username = strings.TrimSuffix(username, ".png") // Allow .png extension

	if username == "" || !validUser.MatchString(username) {
		http.Error(w, "Invalid username", http.StatusBadRequest)
		return
	}

	// Get contributor stats
	stats := getContributorStats(username)
	if stats.totalPRs == 0 {
		http.Error(w, "Contributor not found", http.StatusNotFound)
		return
	}

	// Generate the card image
	img := generateCard(username, stats)

	// Set headers and write PNG
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600") // Cache for 1 hour
	png.Encode(w, img)
}

type contributorStats struct {
	name       string
	totalPRs   int
	releases   int
	avatarURL  string
}

func getContributorStats(username string) contributorStats {
	versions := scraper.GetAvailableVersions()
	stats := contributorStats{}
	releaseSet := make(map[string]bool)

	for _, v := range versions {
		rel, ok := scraper.GetRelease(v.ID)
		if !ok {
			continue
		}
		for _, c := range rel.Contributors {
			if strings.EqualFold(c.GitHubUser, username) {
				stats.totalPRs += len(c.PRs)
				releaseSet[v.ID] = true
				if c.Name != "" {
					stats.name = c.Name
				}
				if c.AvatarURL != "" {
					stats.avatarURL = c.AvatarURL
				}
			}
		}
	}

	stats.releases = len(releaseSet)
	if stats.name == "" {
		stats.name = username
	}

	return stats
}

func generateCard(username string, stats contributorStats) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, cardWidth, cardHeight))

	// Fill background
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)

	// Draw header bar (VS Code blue gradient effect using solid color)
	headerRect := image.Rect(0, 0, cardWidth, 120)
	draw.Draw(img, headerRect, &image.Uniform{primaryColor}, image.Point{}, draw.Src)

	// Draw VS Code logo area (simplified - blue box with text representation)
	logoRect := image.Rect(40, 30, 100, 90)
	draw.Draw(img, logoRect, &image.Uniform{accentColor}, image.Point{}, draw.Src)

	// Draw "VS CODE" text area in header
	drawText(img, 120, 55, "VS CODE", textColor, 3)
	drawText(img, 120, 85, "CONTRIBUTORS", secondaryText, 2)

	// Draw avatar placeholder (circle approximated as rounded rect)
	avatarX, avatarY := 100, 200
	avatarSize := 180
	drawCircle(img, avatarX+avatarSize/2, avatarY+avatarSize/2, avatarSize/2, avatarBgColor)
	// Draw initials in avatar
	initials := getInitials(stats.name)
	drawText(img, avatarX+avatarSize/2-len(initials)*15, avatarY+avatarSize/2+10, initials, textColor, 4)

	// Draw username and name
	drawText(img, 320, 230, stats.name, textColor, 4)
	drawText(img, 320, 280, "@"+username, secondaryText, 2)

	// Draw stats boxes
	statsY := 350

	// PRs box
	drawStatBox(img, 320, statsY, "PULL REQUESTS", stats.totalPRs, primaryColor)

	// Releases box
	drawStatBox(img, 600, statsY, "RELEASES", stats.releases, accentColor)

	// Draw bottom branding bar
	bottomRect := image.Rect(0, cardHeight-60, cardWidth, cardHeight)
	draw.Draw(img, bottomRect, &image.Uniform{color.RGBA{R: 20, G: 20, B: 30, A: 255}}, image.Point{}, draw.Src)

	drawText(img, 40, cardHeight-25, "github.com/microsoft/vscode", secondaryText, 1)

	return img
}

func drawStatBox(img *image.RGBA, x, y int, label string, value int, boxColor color.RGBA) {
	// Draw box background
	boxWidth, boxHeight := 240, 120
	boxRect := image.Rect(x, y, x+boxWidth, y+boxHeight)
	draw.Draw(img, boxRect, &image.Uniform{boxColor}, image.Point{}, draw.Src)

	// Draw value (large)
	valueStr := formatNumber(value)
	drawText(img, x+boxWidth/2-len(valueStr)*20, y+50, valueStr, textColor, 5)

	// Draw label (small)
	drawText(img, x+boxWidth/2-len(label)*5, y+90, label, textColor, 1)
}

func drawCircle(img *image.RGBA, cx, cy, radius int, c color.Color) {
	for y := cy - radius; y <= cy+radius; y++ {
		for x := cx - radius; x <= cx+radius; x++ {
			dx := x - cx
			dy := y - cy
			if dx*dx+dy*dy <= radius*radius {
				img.Set(x, y, c)
			}
		}
	}
}

// drawText draws text using simple pixel patterns (no external fonts)
func drawText(img *image.RGBA, x, y int, text string, c color.Color, scale int) {
	// Simple 5x7 bitmap font for basic characters
	charWidth := 6 * scale
	for i, char := range strings.ToUpper(text) {
		drawChar(img, x+i*charWidth, y, char, c, scale)
	}
}

func drawChar(img *image.RGBA, x, y int, char rune, c color.Color, scale int) {
	// Simplified bitmap patterns for common characters
	patterns := map[rune][]string{
		'A': {"01110", "10001", "11111", "10001", "10001"},
		'B': {"11110", "10001", "11110", "10001", "11110"},
		'C': {"01111", "10000", "10000", "10000", "01111"},
		'D': {"11110", "10001", "10001", "10001", "11110"},
		'E': {"11111", "10000", "11110", "10000", "11111"},
		'F': {"11111", "10000", "11110", "10000", "10000"},
		'G': {"01111", "10000", "10011", "10001", "01110"},
		'H': {"10001", "10001", "11111", "10001", "10001"},
		'I': {"11111", "00100", "00100", "00100", "11111"},
		'J': {"00111", "00010", "00010", "10010", "01100"},
		'K': {"10001", "10010", "11100", "10010", "10001"},
		'L': {"10000", "10000", "10000", "10000", "11111"},
		'M': {"10001", "11011", "10101", "10001", "10001"},
		'N': {"10001", "11001", "10101", "10011", "10001"},
		'O': {"01110", "10001", "10001", "10001", "01110"},
		'P': {"11110", "10001", "11110", "10000", "10000"},
		'Q': {"01110", "10001", "10101", "10010", "01101"},
		'R': {"11110", "10001", "11110", "10010", "10001"},
		'S': {"01111", "10000", "01110", "00001", "11110"},
		'T': {"11111", "00100", "00100", "00100", "00100"},
		'U': {"10001", "10001", "10001", "10001", "01110"},
		'V': {"10001", "10001", "10001", "01010", "00100"},
		'W': {"10001", "10001", "10101", "11011", "10001"},
		'X': {"10001", "01010", "00100", "01010", "10001"},
		'Y': {"10001", "01010", "00100", "00100", "00100"},
		'Z': {"11111", "00010", "00100", "01000", "11111"},
		'0': {"01110", "10011", "10101", "11001", "01110"},
		'1': {"00100", "01100", "00100", "00100", "01110"},
		'2': {"01110", "10001", "00110", "01000", "11111"},
		'3': {"11110", "00001", "01110", "00001", "11110"},
		'4': {"10001", "10001", "11111", "00001", "00001"},
		'5': {"11111", "10000", "11110", "00001", "11110"},
		'6': {"01110", "10000", "11110", "10001", "01110"},
		'7': {"11111", "00001", "00010", "00100", "00100"},
		'8': {"01110", "10001", "01110", "10001", "01110"},
		'9': {"01110", "10001", "01111", "00001", "01110"},
		'@': {"01110", "10001", "10111", "10110", "01111"},
		' ': {"00000", "00000", "00000", "00000", "00000"},
		'-': {"00000", "00000", "11111", "00000", "00000"},
		'_': {"00000", "00000", "00000", "00000", "11111"},
		'.': {"00000", "00000", "00000", "00000", "00100"},
		'/': {"00001", "00010", "00100", "01000", "10000"},
		':': {"00000", "00100", "00000", "00100", "00000"},
	}

	pattern, ok := patterns[char]
	if !ok {
		// Draw a placeholder box for unknown characters
		pattern = []string{"11111", "10001", "10001", "10001", "11111"}
	}

	for row, line := range pattern {
		for col, bit := range line {
			if bit == '1' {
				for sy := 0; sy < scale; sy++ {
					for sx := 0; sx < scale; sx++ {
						img.Set(x+col*scale+sx, y+row*scale+sy, c)
					}
				}
			}
		}
	}
}

func getInitials(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "?"
	}
	if len(parts) == 1 {
		if len(parts[0]) > 0 {
			return strings.ToUpper(string(parts[0][0]))
		}
		return "?"
	}
	first := string(parts[0][0])
	last := string(parts[len(parts)-1][0])
	return strings.ToUpper(first + last)
}

func formatNumber(n int) string {
	if n >= 1000 {
		return strings.TrimSuffix(strings.TrimSuffix(
			strings.Replace(string(rune('0'+n/1000))+"K", "0K", "", 1),
			"K"), "") + "K"
	}
	// Convert int to string manually
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
