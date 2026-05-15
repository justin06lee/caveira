package fabricate

import (
	"context"
	"fmt"
	"io"
	"math/rand"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/justin06lee/caveira/internal/fabricate/llm"
	"github.com/justin06lee/caveira/internal/walk"
)

// maxLLMAttempts is the number of provider calls tried before a hard error.
const maxLLMAttempts = 3

// promptByteBudget caps the total file-content bytes placed in the prompt.
const promptByteBudget = 200_000

// GenerateLLM runs an LLM provider engine: it walks the source HEAD tree,
// segments every file, prompts the provider, parses and realizes the returned
// plan into a base sequence, then reshapes it per mode ("single"/"pigs"/"rats")
// and returns the Plan plus its walk.DAG. On provider, parse, or validation
// failure it retries up to maxLLMAttempts times, then returns a hard error.
func GenerateLLM(repo *git.Repository, ids []Identity, mode string,
	provider llm.Provider, rng *rand.Rand) (*Plan, *walk.DAG, error) {

	if len(ids) == 0 {
		return nil, nil, fmt.Errorf("GenerateLLM: at least one identity required")
	}
	files, err := WalkHead(repo)
	if err != nil {
		return nil, nil, err
	}
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("GenerateLLM: source repo has no files")
	}

	// Read content and segment every file once.
	sources := make([]SourceFile, 0, len(files))
	inputs := make([]llm.FileInput, 0, len(files))
	for _, f := range files {
		blob, err := repo.BlobObject(f.Blob)
		if err != nil {
			return nil, nil, err
		}
		content, err := blobBytes(blob)
		if err != nil {
			return nil, nil, err
		}
		segs := SplitSegments(content)
		sources = append(sources, SourceFile{
			Path: f.Path, Mode: f.Mode, Content: content, Segments: segs,
		})
		si := make([]llm.SegmentInfo, len(segs))
		for i, s := range segs {
			si[i] = llm.SegmentInfo{Index: s.Index, StartLine: s.StartLine, EndLine: s.EndLine}
		}
		inputs = append(inputs, llm.FileInput{
			Path: f.Path, Kind: kindString(Classify(f.Path)), Content: string(content), Segments: si,
		})
	}
	prompt := llm.BuildPrompt(inputs, promptByteBudget)

	var base []SynthCommit
	var lastErr error
	for attempt := 1; attempt <= maxLLMAttempts; attempt++ {
		raw, err := provider.GeneratePlan(context.Background(), prompt)
		if err != nil {
			lastErr = err
			continue
		}
		parsed, err := llm.ParsePlan(raw)
		if err != nil {
			lastErr = err
			continue
		}
		realized, err := Realize(sources, parsed)
		if err != nil {
			lastErr = err
			continue
		}
		base = realized
		lastErr = nil
		break
	}
	if lastErr != nil {
		return nil, nil, fmt.Errorf("LLM engine %q failed after %d attempts: %w",
			provider.Name(), maxLLMAttempts, lastErr)
	}

	plan, err := reshapeBase(base, ids, mode, rng)
	if err != nil {
		return nil, nil, err
	}
	dag, err := PlanToDAG(repo, plan)
	if err != nil {
		return nil, nil, err
	}
	return plan, dag, nil
}

// reshapeBase applies the mode reshaper to a base sequence.
func reshapeBase(base []SynthCommit, ids []Identity, mode string, rng *rand.Rand) (*Plan, error) {
	switch mode {
	case "rats":
		return reshapeRats(base, ids, rng)
	case "pigs":
		return reshapePigs(base, ids, rng), nil
	default: // "single"
		return reshapeSingle(base, ids[0]), nil
	}
}

// reshapeSingle assigns one identity and linear IDs/parents to a base
// sequence, mutating the caller's base slice elements in place.
func reshapeSingle(base []SynthCommit, id Identity) *Plan {
	for i := range base {
		base[i].ID = i
		base[i].Author = id
		base[i].Committer = id
		if i == 0 {
			base[i].Parents = nil
		} else {
			base[i].Parents = []int{i - 1}
		}
	}
	return &Plan{
		Commits: base,
		Refs:    map[string]int{defaultBranch: base[len(base)-1].ID},
		HEAD:    base[len(base)-1].ID,
		HeadRef: defaultBranch,
	}
}

func kindString(k FileKind) string {
	switch k {
	case Chore:
		return "chore"
	case Test:
		return "test"
	default:
		return "code"
	}
}

func blobBytes(blob *object.Blob) ([]byte, error) {
	r, err := blob.Reader()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
