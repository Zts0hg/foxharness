package llmconfig

import "testing"

func TestImageInputProfileDefaultsAndValidation(t *testing.T) {
	for _, mode := range []string{"", "auto", "enabled", "disabled", "invalid"} {
		settings := Settings{DefaultProvider: "p", Providers: map[string]Profile{"p": {Protocol: "openai", BaseURL: "http://localhost", Model: "m", Auth: "none", ImageInput: mode}}}
		got, err := Resolve(settings, EnvOverrides{}, CLIOverrides{}, nil)
		if mode == "invalid" {
			if err == nil {
				t.Fatal("accepted invalid image capability")
			}
			continue
		}
		want := mode
		if want == "" {
			want = "auto"
		}
		if err != nil || got.ImageInput != want {
			t.Fatalf("mode=%q got=%q err=%v", mode, got.ImageInput, err)
		}
	}
}
