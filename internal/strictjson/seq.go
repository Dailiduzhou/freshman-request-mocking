// Package strictjson 实现题目要求的严格 JSON 请求体校验。
//
// 标准库与常见绑定库的默认行为无法满足判题要求：
// 重复字段会被静默覆盖、未知字段被忽略、尾随的第二个 JSON 值不报错。
// 本包基于 json.Decoder 的 token 流逐项检查：
//
//   - 请求体必须为仅含一个 "seq" 字段的 JSON 对象；
//   - 拒绝字符串、布尔值、null、小数、指数形式、数组、对象作为值；
//   - 拒绝重复字段、未知字段、多个 JSON 值及尾随非空白内容；
//   - seq 必须为 1～10 的整数。
package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// MaxBodyBytes 限制请求体长度，超长请求在读取阶段即拒绝。
const MaxBodyBytes = 1024

var ErrEmptyBody = errors.New("请求体为空")

// ParseSeq 解析并校验 {"seq": <int>}，返回 1～10 的编号。
// 所有违规情况均返回非 nil 错误，调用方据此返回 400。
func ParseSeq(body []byte) (int, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return 0, ErrEmptyBody
	}
	if len(body) > MaxBodyBytes {
		return 0, errors.New("请求体过大")
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	tok, err := dec.Token()
	if err != nil {
		return 0, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return 0, errors.New("请求体应为 JSON 对象")
	}

	seenSeq := false
	var seq int
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return 0, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return 0, errors.New("对象键必须为字符串")
		}
		if key != "seq" {
			return 0, fmt.Errorf("未知字段 %q", key)
		}
		if seenSeq {
			return 0, errors.New("字段 seq 重复")
		}
		seenSeq = true

		valTok, err := dec.Token()
		if err != nil {
			return 0, err
		}
		num, ok := valTok.(json.Number)
		if !ok {
			// 字符串、布尔、null、数组、对象均在此被拒绝。
			return 0, errors.New("seq 必须为 1～10 的整数")
		}
		if !isIntegerLiteral(num.String()) {
			// "1.5"、"1e2" 之类的非整数数字面量。
			return 0, errors.New("seq 必须为整数")
		}
		n, err := num.Int64()
		if err != nil {
			return 0, errors.New("seq 必须为整数")
		}
		if n < 1 || n > 10 {
			return 0, errors.New("seq 必须在 1～10 之间")
		}
		seq = int(n)
	}

	// 消费闭合的 '}'；缺失或格式错误都会在这里报错。
	if _, err := dec.Token(); err != nil {
		return 0, err
	}
	// 之后必须只剩空白：任何尾随内容（包括第二个 JSON 值）都拒绝。
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return 0, errors.New("请求体包含多余内容")
	}
	if !seenSeq {
		return 0, errors.New("缺少 seq 字段")
	}
	return seq, nil
}

func isIntegerLiteral(s string) bool {
	for _, c := range s {
		if c == '.' || c == 'e' || c == 'E' {
			return false
		}
	}
	return true
}
