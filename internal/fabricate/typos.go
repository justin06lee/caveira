package fabricate

import "math/rand"

// ApplyTypos returns msg with 0, 1, or 2 typo transformations applied:
//   - 70% probability: 0 typos
//   - 25% probability: 1 typo
//   - 5%  probability: 2 typos
//
// Each typo is one of four transformations, picked uniformly:
//   - adjacent character swap
//   - character drop
//   - character double
//   - keyboard-neighbor substitution (QWERTY)
//
// Positions are chosen uniformly across the message. Pure ASCII assumed; the
// QWERTY substitution table covers the lowercase alphabet.
func ApplyTypos(msg string, rng *rand.Rand) string {
	if msg == "" {
		return msg
	}
	n := drawTypoCount(rng)
	out := msg
	for i := 0; i < n; i++ {
		out = applyOneTypo(out, rng)
	}
	return out
}

func drawTypoCount(rng *rand.Rand) int {
	v := rng.Float64()
	switch {
	case v < 0.70:
		return 0
	case v < 0.95:
		return 1
	default:
		return 2
	}
}

func applyOneTypo(s string, rng *rand.Rand) string {
	if len(s) == 0 {
		return s
	}
	choice := rng.Intn(4)
	switch choice {
	case 0:
		return typoSwap(s, rng)
	case 1:
		return typoDrop(s, rng)
	case 2:
		return typoDouble(s, rng)
	default:
		return typoNeighbor(s, rng)
	}
}

func typoSwap(s string, rng *rand.Rand) string {
	if len(s) < 2 {
		return s
	}
	i := rng.Intn(len(s) - 1)
	b := []byte(s)
	b[i], b[i+1] = b[i+1], b[i]
	return string(b)
}

func typoDrop(s string, rng *rand.Rand) string {
	if len(s) < 2 {
		return s
	}
	i := rng.Intn(len(s))
	return s[:i] + s[i+1:]
}

func typoDouble(s string, rng *rand.Rand) string {
	i := rng.Intn(len(s))
	return s[:i+1] + s[i:i+1] + s[i+1:]
}

var keyboardNeighbors = map[byte]string{
	'a': "qwsz", 'b': "vghn", 'c': "xdfv", 'd': "serfcx",
	'e': "wsdr", 'f': "drtgvc", 'g': "frtyhbv", 'h': "gyujnb",
	'i': "ujko", 'j': "huikmn", 'k': "jilom", 'l': "kop",
	'm': "njk", 'n': "bhjm", 'o': "iklp", 'p': "ol",
	'q': "wa", 'r': "edft", 's': "awedxz", 't': "rfgy",
	'u': "yhji", 'v': "cfgb", 'w': "qase", 'x': "zsdc",
	'y': "tghu", 'z': "asx",
}

func typoNeighbor(s string, rng *rand.Rand) string {
	if len(s) == 0 {
		return s
	}
	for attempt := 0; attempt < 5; attempt++ {
		i := rng.Intn(len(s))
		c := s[i]
		if neighbors, ok := keyboardNeighbors[c]; ok {
			n := neighbors[rng.Intn(len(neighbors))]
			return s[:i] + string(n) + s[i+1:]
		}
	}
	return s
}
