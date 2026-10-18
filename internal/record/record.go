// Package record define o registro de nascimento, comum à base de referência
// e à verdade do conjunto sintético.
package record

import (
	"time"

	"github.com/Fontoura21/ocr2act-go/internal/ids"
)

// Record é um registro de nascimento. Os nomes seguem o contrato do Mini-CIC
// e as datas estão em ISO 8601.
type Record struct {
	Matricula      string `json:"matricula"`
	CPF            string `json:"cpf"`
	Nome           string `json:"nome"`
	DataNascimento string `json:"data_nascimento"`
	HoraNascimento string `json:"hora_nascimento"`
	Sexo           string `json:"sexo"`
	Municipio      string `json:"municipio"`
	UF             string `json:"uf"`
	Mae            string `json:"mae"`
	Pai            string `json:"pai"`
	DataRegistro   string `json:"data_registro"`
	CNS            string `json:"cns"`
}

// Raw devolve o valor bruto de um campo do template. O campo composto
// municipio_uf não tem valor bruto único e devolve ok == false.
func (r Record) Raw(field string) (value string, ok bool) {
	switch field {
	case "nome":
		return r.Nome, true
	case "matricula":
		return r.Matricula, true
	case "cpf":
		return r.CPF, true
	case "data_nascimento":
		return r.DataNascimento, true
	case "hora_nascimento":
		return r.HoraNascimento, true
	case "sexo":
		return r.Sexo, true
	case "mae":
		return r.Mae, true
	case "pai":
		return r.Pai, true
	case "data_registro":
		return r.DataRegistro, true
	case "cns":
		return r.CNS, true
	}
	return "", false
}

// Display devolve o valor do campo como aparece impresso na certidão.
func (r Record) Display(field string) string {
	switch field {
	case "cpf":
		return ids.FormatCPF(r.CPF)
	case "matricula":
		return ids.FormatMatricula(r.Matricula)
	case "data_nascimento":
		return brazilianDate(r.DataNascimento)
	case "data_registro":
		return brazilianDate(r.DataRegistro)
	case "municipio_uf":
		return r.Municipio + "/" + r.UF
	}
	v, _ := r.Raw(field)
	return v
}

// brazilianDate converte AAAA-MM-DD em DD/MM/AAAA. Um valor fora do formato
// volta como veio, para que a divergência apareça na revisão.
func brazilianDate(iso string) string {
	t, err := time.Parse(time.DateOnly, iso)
	if err != nil {
		return iso
	}
	return t.Format("02/01/2006")
}
