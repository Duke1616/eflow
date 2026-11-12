package rule

import "strings"

// toExprText 把画布的 SQL 方言适配成 expr 语法：剥掉变量前缀 $、单等号补成 ==、
// in (...) 的圆括号换成 expr 认的方括号、集合算子归一成小写。其余字符（含空白）一律照抄。
//
// 只做这一层方言适配，分词与语法分析全部交给 expr 的解析器：优先级、括号配对、
// 字面量转义、Unicode 字段名都由它保证，不必在这里手写 tokenizer 与递归下降。
// 单趟扫描且引号内原样抄写，因此字面量不可能被改写——历史上正是正则二次改写把 'a=b'、'$5' 静默篡改掉的。
func toExprText(raw string) string {
	var b strings.Builder
	var brackets []bool // true 表示该层是 in 的集合括号，闭合时还原成 ]
	var word string     // 最近一个标识符，用于识别 in 后面的集合括号

	for i := 0; i < len(raw); {
		c := raw[i]
		switch {
		case c == '\'' || c == '"':
			j := scanLiteral(raw, i)
			b.WriteString(raw[i:j])
			i, word = j, ""

		case c == '$':
			i++ // 变量前缀直接丢弃，字段名紧随其后

		case c == '>' || c == '<' || c == '!':
			b.WriteByte(c)
			i++
			if i < len(raw) && raw[i] == '=' {
				b.WriteByte('=')
				i++
			}
			word = ""

		case c == '=':
			b.WriteString("==") // 单等号与双等号同义
			i++
			if i < len(raw) && raw[i] == '=' {
				i++
			}
			word = ""

		case c == '(' || c == '[':
			isSet := c == '[' || word == "in"
			brackets = append(brackets, isSet)
			if isSet {
				b.WriteByte('[')
			} else {
				b.WriteByte('(')
			}
			i, word = i+1, ""

		case c == ')' || c == ']':
			isSet := false
			if n := len(brackets); n > 0 {
				isSet = brackets[n-1]
				brackets = brackets[:n-1]
			}
			if isSet {
				b.WriteByte(']')
			} else {
				b.WriteByte(')')
			}
			i, word = i+1, ""

		case isIdentPart(c):
			j := i
			for j < len(raw) && isIdentPart(raw[j]) {
				j++
			}
			word = raw[i:j]
			if isSetOperator(word, raw[j:]) {
				word = strings.ToLower(word)
			}
			b.WriteString(word)
			i = j

		default:
			b.WriteByte(c)
			i++
			if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
				word = ""
			}
		}
	}

	return b.String()
}

// isSetOperator 判断刚扫出的标识符是否处在集合算子位置：in 紧跟集合括号，或 not in 紧跟集合括号。
// expr 只认小写 in / not in，而老画布会写 SQL 风格的大写 IN，不归一就会编译失败静默判不命中。
// 只在这个位置归一，名为 In 的普通字段不会被误改成关键字。
func isSetOperator(word, rest string) bool {
	if !strings.EqualFold(word, "in") && !strings.EqualFold(word, "not") {
		return false
	}
	rest = strings.TrimLeft(rest, " \t\r\n")
	if strings.EqualFold(word, "not") {
		n := 0
		for n < len(rest) && isIdentPart(rest[n]) {
			n++
		}
		if !strings.EqualFold(rest[:n], "in") {
			return false
		}
		rest = strings.TrimLeft(rest[n:], " \t\r\n")
	}
	return len(rest) > 0 && (rest[0] == '(' || rest[0] == '[')
}

// scanLiteral 返回字面量的结束下标（含闭合引号）。未闭合时吞到结尾，
// 交给 expr 报错，不在这里 panic。
func scanLiteral(s string, start int) int {
	quote := s[start]
	for j := start + 1; j < len(s); j++ {
		if s[j] == '\\' {
			j++
			continue
		}
		if s[j] == quote {
			return j + 1
		}
	}
	return len(s)
}

// isIdentPart 放行高位字节，让中文字段名与历史解析器一样能被识别
func isIdentPart(c byte) bool {
	return c == '_' || c >= 0x80 || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
