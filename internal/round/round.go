// Package round arredonda números para gravação nos resultados e métricas.
package round

import "strconv"

// To arredonda x para digits casas decimais como o round(x, n) do Python:
// sobre o valor binário exato, com empate para o dígito par. Assim 2.675
// vira 2.67, porque o double mais próximo de 2.675 fica abaixo dele.
func To(x float64, digits int) float64 {
	v, err := strconv.ParseFloat(strconv.FormatFloat(x, 'f', digits, 64), 64)
	if err != nil {
		return x // só acontece com NaN e infinito, que voltam como vieram
	}
	return v
}
