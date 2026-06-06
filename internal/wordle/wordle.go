// Package wordle holds pure Wordle game logic: scoring with correct
// duplicate-letter handling, candidate filtering, and information gain.
// No external dependencies. See ARCHITECTURE.md §9.
package wordle

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

// Game holds the state of a single Wordle game.
type Game struct {
	Answer    string
	Guesses   []string
	Feedbacks []Feedback
	Won       bool
	MaxTurns  int
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
//
// TODO(backend): implement per the spec; QA owns the golden tests.
func ScoreGuess(guess, answer string) Feedback {
	var f Feedback
	return f
}

// WordLists holds the answer pool and the valid-guess set.
type WordLists struct {
	Answers []string
	Guesses map[string]struct{}
}

// LoadWordLists reads answers and guesses (one lowercase word per line).
// TODO(backend): implement file loading + validation.
func LoadWordLists(answersPath, guessesPath string) (*WordLists, error) {
	return &WordLists{Guesses: map[string]struct{}{}}, nil
}

// IsValidGuess reports whether word is in the valid-guess set.
func (wl *WordLists) IsValidGuess(word string) bool {
	_, ok := wl.Guesses[word]
	return ok
}

// FilterCandidates returns the subset of candidates consistent with fb for the
// given guess (i.e. ScoreGuess(guess, c) == fb). Basis for information gain.
// TODO(backend): implement.
func FilterCandidates(candidates []string, guess string, fb Feedback) []string {
	return nil
}

// InfoGainBits returns log2(before/after), the bits eliminated by a guess.
// See ARCHITECTURE.md §9.3 for boundary behavior (before==0, after==0,
// after==before).
// TODO(backend): implement.
func InfoGainBits(before, after int) float64 {
	return 0
}
