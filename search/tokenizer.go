package search

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	MaxTokens   = 64
	eosID       = 1
	padID       = 1
	unkPenalty  = 10.0
	metaspace   = "▁"
	punctuation = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"
)

type Tokenizer struct {
	pieces   map[string]int32
	scores   []float32
	unkID    int32
	unkScore float64
	maxLen   int
}

type vocabEntry struct {
	Piece string
	Score float64
}

func (v *vocabEntry) UnmarshalJSON(b []byte) error {
	var raw [2]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if err := json.Unmarshal(raw[0], &v.Piece); err != nil {
		return err
	}
	return json.Unmarshal(raw[1], &v.Score)
}

func LoadTokenizer(path string) (*Tokenizer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var doc struct {
		Model struct {
			Type  string       `json:"type"`
			UnkID int32        `json:"unk_id"`
			Vocab []vocabEntry `json:"vocab"`
		} `json:"model"`
	}
	if err := json.NewDecoder(f).Decode(&doc); err != nil {
		return nil, err
	}
	if doc.Model.Type != "Unigram" {
		return nil, fmt.Errorf("tokenizer %q not supported", doc.Model.Type)
	}

	t := &Tokenizer{
		pieces: make(map[string]int32, len(doc.Model.Vocab)),
		scores: make([]float32, len(doc.Model.Vocab)),
		unkID:  doc.Model.UnkID,
	}
	minScore := math.Inf(1)
	for i, v := range doc.Model.Vocab {
		t.scores[i] = float32(v.Score)
		if i <= 2 || isByteToken(v.Piece) {
			continue
		}
		t.pieces[v.Piece] = int32(i)
		t.maxLen = max(t.maxLen, utf8.RuneCountInString(v.Piece))
		minScore = min(minScore, v.Score)
	}
	t.unkScore = minScore - unkPenalty
	return t, nil
}

func isByteToken(s string) bool {
	return len(s) == 6 && strings.HasPrefix(s, "<0x") && strings.HasSuffix(s, ">")
}

func Canonicalize(text string) string {
	text = norm.NFKC.String(text)
	text = strings.ToLower(text)
	var b strings.Builder
	space := true
	for _, r := range text {
		switch {
		case strings.ContainsRune(punctuation, r), unicode.IsControl(r):
			continue
		case unicode.IsSpace(r):
			if !space {
				b.WriteByte(' ')
			}
			space = true
		default:
			b.WriteRune(r)
			space = false
		}
	}
	return strings.TrimSpace(b.String())
}

func (t *Tokenizer) Tokenize(text string) []int32 {
	text = Canonicalize(text)
	var ids []int32
	if text == "" {
		return ids
	}
	for _, word := range strings.Split(text, " ") {
		ids = append(ids, t.viterbi(metaspace+word)...)
	}
	return ids
}

func (t *Tokenizer) Encode(text string) []int64 {
	ids := t.Tokenize(text)
	if len(ids) > MaxTokens-1 {
		ids = ids[:MaxTokens-1]
	}
	out := make([]int64, MaxTokens)
	for i := range out {
		out[i] = padID
	}
	for i, id := range ids {
		out[i] = int64(id)
	}
	out[len(ids)] = eosID
	return out
}

func (t *Tokenizer) viterbi(s string) []int32 {
	bounds := []int{0}
	for i := range s {
		if i > 0 {
			bounds = append(bounds, i)
		}
	}
	bounds = append(bounds, len(s))
	n := len(bounds) - 1

	best := make([]float64, n+1)
	from := make([]int, n+1)
	token := make([]int32, n+1)
	for i := 1; i <= n; i++ {
		best[i] = math.Inf(-1)
	}

	for start := 0; start < n; start++ {
		if math.IsInf(best[start], -1) {
			continue
		}
		single := false
		for end := start + 1; end <= n && end-start <= t.maxLen; end++ {
			id, ok := t.pieces[s[bounds[start]:bounds[end]]]
			if !ok {
				continue
			}
			if end == start+1 {
				single = true
			}
			if score := best[start] + float64(t.scores[id]); score > best[end] {
				best[end] = score
				from[end] = start
				token[end] = id
			}
		}
		if !single {
			if score := best[start] + t.unkScore; score > best[start+1] {
				best[start+1] = score
				from[start+1] = start
				token[start+1] = t.unkID
			}
		}
	}

	var ids []int32
	for i := n; i > 0; i = from[i] {
		if token[i] == t.unkID && len(ids) > 0 && ids[len(ids)-1] == t.unkID {
			continue
		}
		ids = append(ids, token[i])
	}
	for l, r := 0, len(ids)-1; l < r; l, r = l+1, r-1 {
		ids[l], ids[r] = ids[r], ids[l]
	}
	return ids
}
