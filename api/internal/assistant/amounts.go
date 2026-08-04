package assistant

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var conversationalAmountPattern = regexp.MustCompile(`(?i)(?:\$|usd|d[oó]lares?|dollars?)\s*([0-9]+(?:[.,][0-9]{1,2})?)`)
var decimalAmountPattern = regexp.MustCompile(`(?i)\b[0-9]+[.,][0-9]{1,2}\b`)

// parseConversationalAmount keeps the public assistant language friendly
// while returning the integer minor-unit representation required by MCP.
// Bare integers preserve the legacy contract: "depositar 2500" means 2500
// minor units. Currency-marked or decimal values are interpreted as dollars.
func parseConversationalAmount(message string) (string, error) {
	clean := uuidPattern.ReplaceAllString(strings.ToLower(message), " ")
	if match := conversationalAmountPattern.FindStringSubmatch(clean); len(match) == 2 {
		return dollarsToMinor(match[1])
	}
	if match := decimalAmountPattern.FindString(clean); match != "" {
		return dollarsToMinor(match)
	}
	if words, ok := spanishNumberWords(clean); ok {
		return strconv.FormatInt(words*100, 10), nil
	}
	if match := amountPattern.FindString(clean); match != "" {
		if _, err := strconv.ParseInt(match, 10, 64); err != nil {
			return "", fmt.Errorf("el monto no es válido")
		}
		return match, nil
	}
	return "", fmt.Errorf("indica un monto, por ejemplo 2500 o USD 25.00")
}

func dollarsToMinor(value string) (string, error) {
	value = strings.Replace(strings.TrimSpace(value), ",", ".", 1)
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" || len(parts) == 2 && len(parts[1]) > 2 {
		return "", fmt.Errorf("el monto no es válido")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 63)
	if err != nil || whole < 0 {
		return "", fmt.Errorf("el monto no es válido")
	}
	minor := int64(0)
	if len(parts) == 2 {
		fraction := parts[1]
		if len(fraction) == 1 {
			fraction += "0"
		}
		minor, err = strconv.ParseInt(fraction, 10, 8)
		if err != nil {
			return "", fmt.Errorf("el monto no es válido")
		}
	}
	return strconv.FormatInt(whole*100+minor, 10), nil
}

func spanishNumberWords(message string) (int64, bool) {
	words := strings.Fields(strings.NewReplacer(",", " ", ".", " ", "dólares", " ", "dolares", " ", "dollar", " ", "dollars", " ").Replace(message))
	if len(words) == 0 {
		return 0, false
	}
	units := map[string]int64{"cero": 0, "un": 1, "uno": 1, "una": 1, "dos": 2, "tres": 3, "cuatro": 4, "cinco": 5, "seis": 6, "siete": 7, "ocho": 8, "nueve": 9, "diez": 10, "once": 11, "doce": 12, "trece": 13, "catorce": 14, "quince": 15}
	tens := map[string]int64{"veinte": 20, "treinta": 30, "cuarenta": 40, "cincuenta": 50, "sesenta": 60, "setenta": 70, "ochenta": 80, "noventa": 90}
	hundreds := map[string]int64{"cien": 100, "ciento": 100, "doscientos": 200, "trescientos": 300, "cuatrocientos": 400, "quinientos": 500, "seiscientos": 600, "setecientos": 700, "ochocientos": 800, "novecientos": 900}
	numericWords := make([]string, 0, len(words))
	for _, word := range words {
		if _, ok := units[word]; ok || word == "y" || word == "mil" {
			numericWords = append(numericWords, word)
			continue
		}
		if _, ok := tens[word]; ok {
			numericWords = append(numericWords, word)
			continue
		}
		if _, ok := hundreds[word]; ok {
			numericWords = append(numericWords, word)
		}
	}
	if len(numericWords) == 0 {
		return 0, false
	}
	total, current := int64(0), int64(0)
	valid := false
	for _, word := range numericWords {
		if word == "y" {
			continue
		}
		if value, ok := units[word]; ok {
			current += value
			valid = true
			continue
		}
		if value, ok := tens[word]; ok {
			current += value
			valid = true
			continue
		}
		if value, ok := hundreds[word]; ok {
			current += value
			valid = true
			continue
		}
		if word == "mil" {
			if current == 0 {
				current = 1
			}
			total += current * 1000
			current = 0
			valid = true
			continue
		}
		return 0, false
	}
	return total + current, valid && total+current > 0
}
