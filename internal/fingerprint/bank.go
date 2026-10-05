package fingerprint

import (
	_ "embed"
	"encoding/json"
)

//go:embed bank.generated.json
var bankGenerated []byte

// Bank is the dual fingerprint bank: numeric (ModelTrace) + behavioral (v3e).
type Bank struct {
	Source     map[string]string `json:"source"`
	Numeric    NumericBank       `json:"numeric"`
	Behavioral BehavioralBank    `json:"behavioral"`
}

type NumericBank struct {
	Models              []ModelDef            `json:"models"`
	Robust              Robust                `json:"robust"`
	Calibration         map[string]CalibEntry `json:"calibration"`
	Method              Method                `json:"method"`
	MinimumValidNumbers int                   `json:"minimum_valid_numbers"`
}

type ModelDef struct {
	Aliases          []string `json:"aliases,omitempty"`
	ReferenceSource  string   `json:"reference_source,omitempty"`
	ReferenceSamples int      `json:"reference_samples,omitempty"`
	ID               string   `json:"id"`
	DisplayName      string   `json:"display_name"`
	Family           string   `json:"family"`
	FamilyName       string   `json:"family_name"`
	Counts           []int    `json:"counts"`
}

type Robust struct {
	ModelOrder    []string     `json:"model_order"`
	Hellinger     HellingerArt `json:"hellinger"`
	OrderedBlocks OrderedArt   `json:"ordered_blocks"`
}

type HellingerArt struct {
	FeatureMean   []float64   `json:"feature_mean"`
	FeatureScale  []float64   `json:"feature_scale"`
	NuisanceBasis [][]float64 `json:"nuisance_basis"`
	Centroids     [][]float64 `json:"centroids"`
}

type OrderedArt struct {
	Weight               float64       `json:"weight"`
	FeatureMean          []float64     `json:"feature_mean"`
	FeatureScale         []float64     `json:"feature_scale"`
	NuisanceBasis        [][]float64   `json:"nuisance_basis"`
	Centroids            [][]float64   `json:"centroids"`
	EnvironmentCentroids [][][]float64 `json:"environment_centroids"`
}

type CalibEntry struct {
	Beta       float64 `json:"beta"`
	CVAccuracy float64 `json:"cv_accuracy"`
}

type Method struct {
	Name               string  `json:"name"`
	Range              []int   `json:"range"`
	Alpha              float64 `json:"alpha"`
	OrderedBlockWeight float64 `json:"ordered_block_weight"`
}

type BehavioralBank struct {
	Models []BehavioralModel `json:"models"`
}

type BehavioralModel struct {
	ModelID       string         `json:"modelId"`
	Family        string         `json:"family"`
	DisplayName   string         `json:"displayName"`
	RefusalLadder *RefusalLadder `json:"refusalLadder"`
	Formatting    *Formatting    `json:"formatting"`
	Uncertainty   *Uncertainty   `json:"uncertainty"`
	SampleSize    int            `json:"sampleSize"`
	UpdatedAt     string         `json:"updatedAt"`
}

type RefusalLadder struct {
	VectorAvg        []float64 `json:"vectorAvg"`
	RefusedCountAvg  float64   `json:"refusedCountAvg"`
	AvgRefusalLength float64   `json:"avgRefusalLengthAvg"`
	FirstRefusalRung float64   `json:"firstRefusalRungAvg"`
}

type Formatting struct {
	BulletCharMode string  `json:"bulletCharMode"`
	HeaderDepthAvg float64 `json:"headerDepthAvg"`
	UsesEmDashRate float64 `json:"usesEmDashRate"`
}

type Uncertainty struct {
	ValueAvg    float64 `json:"valueAvg"`
	IsRoundRate float64 `json:"isRoundRate"`
}

// LoadBank parses the embedded generated bank.
func LoadBank() (*Bank, error) {
	var b Bank
	if err := json.Unmarshal(bankGenerated, &b); err != nil {
		return nil, err
	}
	if err := MergeLocalReferences(&b); err != nil {
		return nil, err
	}
	return &b, nil
}
