package mommy

import (
	"encoding/json"
	"fmt"
	"maps"
	"math/rand"
	"time"

	_ "embed"
)

//go:embed responses.json
var responsesJSON []byte

// DefaultResponses is the default responses.json file.
// It is embedded into the binary.
var DefaultResponses Responses

func init() {
	var err error
	DefaultResponses, err = UnmarshalResponses(responsesJSON)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal default responses: %v", err))
	}
}

// Responses is the JSON representation of the responses.json file.
// It is used to unmarshal the responses.json file.
type Responses struct {
	// Moods is a map of moods that mommy has. This map contains the responses
	// that mommy will use when she is in that mood.
	Moods map[Spiciness]Mood `json:"moods"`
	// Vars is a map of variables that can be used in the responses.
	// It is used in tandem with the template strings.
	Vars map[VariableKey]Variable `json:"vars"`
}

// UnmarshalResponses unmarshals the responses.json file.
func UnmarshalResponses(data []byte) (Responses, error) {
	var r Responses
	err := json.Unmarshal(data, &r)
	return r, err
}

// WithVariable sets a variable to a new responses object.
func (r Responses) WithVariable(k VariableKey, v []string) Responses {
	r.Vars = maps.Clone(r.Vars)
	r.Vars[k] = Variable{Defaults: v}
	return r
}

// WithVariables sets variables to a new responses object.
func (r Responses) WithVariables(vars map[VariableKey][]string) Responses {
	r.Vars = maps.Clone(r.Vars)
	for k, v := range vars {
		r.Vars[k] = Variable{Defaults: v}
	}
	return r
}

// Spiciness is a measure of how NSFW a response is.
type Spiciness = string

const (
	Chill   Spiciness = "chill"
	Thirsty Spiciness = "thirsty" // NSFW
	Yikes   Spiciness = "yikes"   // NSFW
)

// IsNSFW returns true if the given spiciness is NSFW.
func IsNSFW(s Spiciness) bool {
	return s != Chill
}

// Template describes a string that contains templating variables.
// It expects the syntax "{variable_name}".
type Template string

// Mood contains the responses for a given mood. It is represented as
// a list of positive and negative responses, and optionally a spiciness
// level.
type Mood map[ResponseType][]Template

// VariableKey is the key for a response variable.
type VariableKey string

const (
	VariableMood             VariableKey = "mood"
	VariableEmote            VariableKey = "emote"
	VariablePronoun          VariableKey = "pronoun"
	VariableRole             VariableKey = "role"
	VariableAffectionateTerm VariableKey = "affectionate_term"
	VariableDenigratingTerm  VariableKey = "denigrating_term" // NSFW
	VariablePart             VariableKey = "part"             // NSFW
)

// Variable is a variable that is used in the string templates.
type Variable struct {
	// Spiciness is a measure of how NSFW this variable is.
	Spiciness Spiciness `json:"spiciness,omitempty"` // NSFW
	// Defaults is a list of default values for this variable.
	Defaults []string `json:"defaults"`
	// EnvKey is the environment variable key that can be used to override the
	// default values. This field is not used by this package.
	EnvKey string `json:"env_key,omitempty"`
}

// ResponseType is the type of response that mommy will generate.
type ResponseType string

const (
	PositiveResponse ResponseType = "positive"
	NegativeResponse ResponseType = "negative"
)

// Generator is a mommy response generator.
type Generator struct {
	Random *rand.Rand

	variables map[VariableKey]Variable
	templates map[templateKey][]templater
}

type templateKey struct {
	Spiciness Spiciness
	Response  ResponseType
}

// NewGenerator creates a new mommy response generator with a new automatically
// seeded random number generator.
func NewGenerator(config Responses) (*Generator, error) {
	random := rand.New(rand.NewSource(time.Now().UnixNano()))
	return NewGeneratorWithRandom(config, random)
}

// NewGeneratorWithRandom creates a new mommy response generator with the given
// random number generator.
func NewGeneratorWithRandom(config Responses, random *rand.Rand) (*Generator, error) {
	g := &Generator{Random: random}
	g.variables = config.Vars
	g.templates = make(map[templateKey][]templater, len(config.Moods))
	for spiciness, mood := range config.Moods {
		for responseType, stringTemplates := range mood {
			templates := make([]templater, len(stringTemplates))
			for i, stringTemplate := range stringTemplates {
				templates[i] = compileTemplate(stringTemplate)
			}
			key := templateKey{spiciness, responseType}
			g.templates[key] = templates
		}
	}

	return g, nil
}

// GenerateOverrides is a map of variable keys to values that can be used to
// override the default values. If a variable is not present in this map, then
// a random default value will be used.
type GenerateOverrides map[VariableKey]string

// Generate generates a response from the given response type with an optional
// set of overrides.
// If a variable is present in the overrides map, then it will use that value.
// If a variable is not present in the overrides map, then it will use a random
// default value.
// If a variable has no default values, then it will return an error
func (g *Generator) Generate(response ResponseType, overrides GenerateOverrides) (string, error) {
	values := make(map[VariableKey]string, len(g.variables))
	for k, v := range g.variables {
		if override, ok := overrides[k]; ok {
			values[k] = override
			continue
		}
		if len(v.Defaults) == 0 {
			return "", fmt.Errorf("no default values for variable %q", k)
		}
		values[k] = v.Defaults[g.Random.Intn(len(v.Defaults))]
	}

	key := templateKey{values[VariableMood], response}

	templates, ok := g.templates[key]
	if !ok {
		return "", fmt.Errorf("no templates for mood %q and response type %q", values[VariableMood], response)
	}

	template := templates[g.Random.Intn(len(templates))]
	return template.render(values), nil
}