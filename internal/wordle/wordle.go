// Package wordle holds pure Wordle game logic: scoring with correct
// duplicate-letter handling, candidate filtering, and information gain.
// No external dependencies. See ARCHITECTURE.md §9.
package wordle

import (
	"bufio"
	"math"
	"os"
	"strings"
)

// TileResult represents the result of a single tile in a Wordle guess.
type TileResult int

const (
	Gray   TileResult = 0 // letter not in word
	Yellow TileResult = 1 // letter in word, wrong position
	Green  TileResult = 2 // letter in word, correct position
)

// WordLen is the fixed Wordle word length.
const WordLen = 5

// MaxTurns is the number of guesses allowed per game.
const MaxTurns = 6

// Feedback is a 5-element array of TileResult.
type Feedback [WordLen]TileResult

// allGreen is the winning feedback pattern.
var allGreen = Feedback{Green, Green, Green, Green, Green}

// String encodes feedback as a GYX string (G=green, Y=yellow, X=gray).
func (f Feedback) String() string {
	b := make([]byte, WordLen)
	for i, t := range f {
		switch t {
		case Green:
			b[i] = 'G'
		case Yellow:
			b[i] = 'Y'
		default:
			b[i] = 'X'
		}
	}
	return string(b)
}

// FromString parses a GYX-encoded string into a Feedback. Invalid characters
// are treated as Gray. If s is not exactly WordLen characters, returns zero.
func FromString(s string) Feedback {
	var f Feedback
	if len(s) != WordLen {
		return f
	}
	for i, c := range s {
		switch c {
		case 'G', 'g':
			f[i] = Green
		case 'Y', 'y':
			f[i] = Yellow
		default:
			f[i] = Gray
		}
	}
	return f
}

// Game holds the state of a single Wordle game.
type Game struct {
	Answer    string
	Guesses   []string
	Feedbacks []Feedback
	Won       bool
	MaxTurns  int
}

// NewGame creates a new game for the given answer with the default turn limit.
func NewGame(answer string) *Game {
	return &Game{
		Answer:   answer,
		MaxTurns: MaxTurns,
	}
}

// NewGameWithMaxTurns creates a new game with a custom turn limit.
func NewGameWithMaxTurns(answer string, maxTurns int) *Game {
	return &Game{
		Answer:   answer,
		MaxTurns: maxTurns,
	}
}

// AddGuess scores a guess, appends it to the game state, and returns the feedback.
// Callers must check IsOver before calling.
func (g *Game) AddGuess(guess string) Feedback {
	fb := ScoreGuess(guess, g.Answer)
	g.Guesses = append(g.Guesses, guess)
	g.Feedbacks = append(g.Feedbacks, fb)
	if fb == allGreen {
		g.Won = true
	}
	return fb
}

// IsWon reports whether the game has been won.
func (g *Game) IsWon() bool { return g.Won }

// IsOver reports whether the game is finished (won or max turns reached).
func (g *Game) IsOver() bool {
	return g.Won || len(g.Guesses) >= g.MaxTurns
}

// ScoreGuess scores a guess against the answer using correct duplicate-letter
// handling. Spec (ARCHITECTURE.md §9.1):
//
//	Pass 1 — assign Green for every exact positional match, decrementing a
//	         per-letter budget built from the answer.
//	Pass 2 — for each remaining (non-green) position left-to-right, assign
//	         Yellow if the guessed letter still has budget, else Gray.
//
// Inputs are assumed to be exactly WordLen lowercase ASCII letters; callers
// must validate upstream. Mismatched lengths return the zero Feedback (XXXXX).
func ScoreGuess(guess, answer string) Feedback {
	var f Feedback
	if len(guess) != WordLen || len(answer) != WordLen {
		return f // all gray (zero value)
	}

	// Build per-letter remaining budget from answer.
	var remaining [26]int
	for i := 0; i < WordLen; i++ {
		remaining[answer[i]-'a']++
	}

	// Pass 1: assign Greens and decrement budget.
	for i := 0; i < WordLen; i++ {
		if guess[i] == answer[i] {
			f[i] = Green
			remaining[guess[i]-'a']--
		}
	}

	// Pass 2: assign Yellows or Grays for non-green positions.
	for i := 0; i < WordLen; i++ {
		if f[i] == Green {
			continue
		}
		idx := guess[i] - 'a'
		if remaining[idx] > 0 {
			f[i] = Yellow
			remaining[idx]--
		} else {
			f[i] = Gray
		}
	}

	return f
}

// WordLists holds the answer pool and the valid-guess set.
type WordLists struct {
	Answers []string
	Guesses map[string]struct{}
}

// LoadWordLists reads answers and guesses (one lowercase word per line).
// Empty lines are skipped; whitespace is trimmed.
func LoadWordLists(answersPath, guessesPath string) (*WordLists, error) {
	answers, err := loadWords(answersPath)
	if err != nil {
		return nil, err
	}

	guessList, err := loadWords(guessesPath)
	if err != nil {
		return nil, err
	}

	guessSet := make(map[string]struct{}, len(guessList))
	for _, w := range guessList {
		guessSet[w] = struct{}{}
	}

	return &WordLists{
		Answers: answers,
		Guesses: guessSet,
	}, nil
}

// loadWords reads one word per line from path, lowercased, trimmed, skipping empty lines.
func loadWords(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var words []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		w := strings.TrimSpace(strings.ToLower(sc.Text()))
		if w != "" {
			words = append(words, w)
		}
	}
	return words, sc.Err()
}

// IsValidGuess reports whether word is in the valid-guess set.
func (wl *WordLists) IsValidGuess(word string) bool {
	_, ok := wl.Guesses[word]
	return ok
}

// FilterCandidates returns the subset of candidates consistent with fb for the
// given guess (i.e. ScoreGuess(guess, c) == fb). Basis for information gain.
// Returns an empty slice (not nil) when candidates is empty.
func FilterCandidates(candidates []string, guess string, fb Feedback) []string {
	out := candidates[:0:0] // empty slice with no backing array
	for _, c := range candidates {
		if ScoreGuess(guess, c) == fb {
			out = append(out, c)
		}
	}
	return out
}

// InfoGainBits returns log2(before/after), the bits eliminated by a guess.
// Boundary behavior (ARCHITECTURE.md §9.3):
//   - before == 0 → 0 (no information definable)
//   - after  == 0 → treat as 1 (defensive; answer is always consistent)
//   - after == before → 0 bits (guess eliminated nothing)
func InfoGainBits(before, after int) float64 {
	if before == 0 {
		return 0
	}
	if after <= 0 {
		after = 1
	}
	if after >= before {
		return 0
	}
	return math.Log2(float64(before) / float64(after))
}
