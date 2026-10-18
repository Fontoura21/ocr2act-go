package record

import "testing"

func TestDisplay(t *testing.T) {
	r := Record{
		Matricula: "83879701552021100036024008667405", CPF: "34131647591",
		Nome: "APOLLO PORTO SOUZA", DataNascimento: "2021-06-22", HoraNascimento: "05:23",
		Sexo: "MASCULINO", Municipio: "DA CRUZ DE GOIÁS", UF: "BA", Mae: "BELLA ROCHA PORTO",
		Pai: "YAGO BORGES CAMARGO", DataRegistro: "2021-07-05", CNS: "838797",
	}
	tests := map[string]string{
		"nome":            "APOLLO PORTO SOUZA",
		"matricula":       "838797 01 55 2021 1 00036 024 0086674 05",
		"cpf":             "341.316.475-91",
		"data_nascimento": "22/06/2021",
		"hora_nascimento": "05:23",
		"sexo":            "MASCULINO",
		"municipio_uf":    "DA CRUZ DE GOIÁS/BA",
		"mae":             "BELLA ROCHA PORTO",
		"pai":             "YAGO BORGES CAMARGO",
		"data_registro":   "05/07/2021",
		"cns":             "838797",
		"desconhecido":    "",
	}
	for field, want := range tests {
		if got := r.Display(field); got != want {
			t.Errorf("Display(%q) = %q, want %q", field, got, want)
		}
	}
	if _, ok := r.Raw("municipio_uf"); ok {
		t.Error("Raw(municipio_uf) deveria não ter valor bruto")
	}
	if got := (Record{DataNascimento: "22/06/2021"}).Display("data_nascimento"); got != "22/06/2021" {
		t.Errorf("data fora do formato ISO deveria voltar como veio, veio %q", got)
	}
}
