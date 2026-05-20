package bridge

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var templateRe = regexp.MustCompile(`\{\{([^}]+)\}\}`)

type TemplateContext struct {
	InputText string
	TaskId    string
	ContextId string
	SkillId   string
}

func renderString(tmpl string, ctx *TemplateContext) string {
	return templateRe.ReplaceAllStringFunc(tmpl, func(match string) string {
		key := strings.TrimSpace(match[2 : len(match)-2])
		switch key {
		case "inputText":
			return ctx.InputText
		case "taskId":
			return ctx.TaskId
		case "contextId":
			return ctx.ContextId
		case "skillId":
			return ctx.SkillId
		}
		return match
	})
}

func renderBody(body interface{}, ctx *TemplateContext) interface{} {
	switch v := body.(type) {
	case string:
		return renderString(v, ctx)
	case map[string]interface{}:
		result := make(map[string]interface{}, len(v))
		for k, val := range v {
			result[k] = renderBody(val, ctx)
		}
		return result
	case map[interface{}]interface{}:
		result := make(map[string]interface{}, len(v))
		for k, val := range v {
			result[fmt.Sprint(k)] = renderBody(val, ctx)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = renderBody(val, ctx)
		}
		return result
	default:
		return v
	}
}

func extractFromResponse(tmpl string, response interface{}) string {
	return templateRe.ReplaceAllStringFunc(tmpl, func(match string) string {
		key := strings.TrimSpace(match[2 : len(match)-2])
		if !strings.HasPrefix(key, "output.") {
			return match
		}
		path := strings.TrimPrefix(key, "output.")
		val := resolvePath(response, strings.Split(path, "."))
		if val == nil {
			return ""
		}
		switch v := val.(type) {
		case string:
			return v
		case float64:
			if v == float64(int64(v)) {
				return strconv.FormatInt(int64(v), 10)
			}
			return strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			return strconv.FormatBool(v)
		default:
			b, _ := json.Marshal(v)
			return string(b)
		}
	})
}

func resolvePath(obj interface{}, parts []string) interface{} {
	if len(parts) == 0 {
		return obj
	}
	key := parts[0]
	rest := parts[1:]

	switch v := obj.(type) {
	case map[string]interface{}:
		return resolvePath(v[key], rest)
	case []interface{}:
		idx, err := strconv.Atoi(key)
		if err != nil || idx < 0 || idx >= len(v) {
			return nil
		}
		return resolvePath(v[idx], rest)
	default:
		return nil
	}
}
