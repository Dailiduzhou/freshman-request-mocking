package strictjson

import "testing"

func TestParseSeqAcceptsValid(t *testing.T) {
	for _, body := range []string{
		`{"seq": 1}`,
		`{"seq":10}`,
		`{"seq": 5}`,
	} {
		seq, err := ParseSeq([]byte(body))
		if err != nil {
			t.Fatalf("%q 应合法: %v", body, err)
		}
		if seq < 1 || seq > 10 {
			t.Fatalf("%q 解析出越界编号 %d", body, seq)
		}
	}
}

func TestParseSeqRejectsInvalid(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"空体", ``},
		{"纯空白", `   `},
		{"字符串值", `{"seq": "1"}`},
		{"布尔值", `{"seq": true}`},
		{"null", `{"seq": null}`},
		{"小数", `{"seq": 1.5}`},
		{"整数形小数", `{"seq": 1.0}`},
		{"指数", `{"seq": 1e2}`},
		{"大指数", `{"seq": 5E1}`},
		{"数组", `[1]`},
		{"对象值", `{"seq": {"a":1}}`},
		{"数组值", `{"seq": [1]}`},
		{"缺少字段", `{}`},
		{"未知字段", `{"seq": 1, "x": 2}`},
		{"重复字段", `{"seq": 1, "seq": 2}`},
		{"多个值", `{"seq": 1} {"seq": 2}`},
		{"尾随垃圾", `{"seq": 1}garbage`},
		{"截断", `{"seq": 1`},
		{"越界0", `{"seq": 0}`},
		{"越界11", `{"seq": 11}`},
		{"负数", `{"seq": -1}`},
		{"纯数字", `1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseSeq([]byte(tc.body)); err == nil {
				t.Fatalf("%q 应被拒绝", tc.body)
			}
		})
	}
}

func TestParseSeqRejectsOversizedBody(t *testing.T) {
	body := make([]byte, MaxBodyBytes+1)
	for i := range body {
		body[i] = ' '
	}
	if _, err := ParseSeq(body); err == nil {
		t.Fatal("超长请求体应被拒绝")
	}
}
