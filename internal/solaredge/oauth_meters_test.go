package solaredge

import (
	"encoding/json"
	"testing"
)

func TestMeterTelemetryMergesPhysicalAndConsumption(t *testing.T) {
	raw := []byte(`{
	  "meters": {
	    "CONSUMPTION": {
	      "consumptionPower": {"unit":"W","values":[
	        {"timestamp":"2026-11-25T17:00:00-08:00","value":700.0},
	        {"timestamp":"2026-11-25T17:15:00-08:00","value":null}
	      ]}
	    },
	    "4147088": {
	      "importPower": {"unit":"W","values":[
	        {"timestamp":"2026-11-25T17:00:00-08:00","value":700.0},
	        {"timestamp":"2026-11-25T17:15:00-08:00","value":null}
	      ]},
	      "exportPower": {"unit":"W","values":[
	        {"timestamp":"2026-11-25T17:00:00-08:00","value":0.0},
	        {"timestamp":"2026-11-25T17:15:00-08:00","value":null}
	      ]}
	    }
	  }
	}`)
	var payload struct {
		Meters map[string]struct {
			ImportPower struct {
				Values telemetryPoints `json:"values"`
			} `json:"importPower"`
			ExportPower struct {
				Values telemetryPoints `json:"values"`
			} `json:"exportPower"`
			ConsumptionPower struct {
				Values telemetryPoints `json:"values"`
			} `json:"consumptionPower"`
		} `json:"meters"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	var importW, exportW, consumptionW float64
	var haveConsumption bool
	for _, m := range payload.Meters {
		if imp := lastTelemetry(m.ImportPower.Values); imp != nil {
			importW = *imp
		}
		if exp := lastTelemetry(m.ExportPower.Values); exp != nil {
			exportW = *exp
		}
		if load := lastTelemetry(m.ConsumptionPower.Values); load != nil {
			consumptionW = *load
			haveConsumption = true
		}
	}
	if importW != 700 {
		t.Fatalf("import=%v want 700", importW)
	}
	if exportW != 0 {
		t.Fatalf("export=%v want 0", exportW)
	}
	if !haveConsumption || consumptionW != 700 {
		t.Fatalf("consumption=%v have=%v want 700", consumptionW, haveConsumption)
	}
}
