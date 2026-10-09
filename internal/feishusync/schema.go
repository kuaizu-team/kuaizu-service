// Package feishusync synchronizes one school's users using application identity.
package feishusync

import (
	"database/sql"
	"fmt"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"math"
	"strconv"
)

type Option struct {
	Name string `json:"name"`
	ID   string `json:"id,omitempty"`
}
type Property struct {
	Options   []Option `json:"options,omitempty"`
	Formatter string   `json:"formatter,omitempty"`
}
type Field struct {
	Name     string    `json:"field_name"`
	Type     int       `json:"type"`
	ID       string    `json:"field_id,omitempty"`
	Hidden   bool      `json:"is_hidden,omitempty"`
	Property *Property `json:"property,omitempty"`
}

// Names, order and meanings are the original user CSV contract. User IDs stay local.
func Fields() []Field {
	selectField := func(name string, options ...string) Field {
		f := Field{Name: name, Type: 3, Property: &Property{}}
		for _, option := range options {
			f.Property.Options = append(f.Property.Options, Option{Name: option})
		}
		return f
	}
	return []Field{
		{Name: "昵称", Type: 1},
		selectField("MBTI", "INTJ", "INTP", "ENTJ", "ENTP", "INFJ", "INFP", "ENFJ", "ENFP", "ISTJ", "ISFJ", "ESTJ", "ESFJ", "ISTP", "ISFP", "ESTP", "ESFP"),
		{Name: "学校", Type: 1}, {Name: "专业", Type: 1}, {Name: "入学年份", Type: 1},
		{Name: "自我介绍", Type: 1}, {Name: "项目经历", Type: 1},
		selectField("协作等级", "极好", "优秀", "良好", "中等", "较差"),
		{Name: "协作具体分数值", Type: 2, Property: &Property{Formatter: "0.00"}},
		selectField("是否通过学生认证", "是", "否"),
		selectField("是否入驻人才库", "已入驻人才库", "未入驻人才库"),
		{Name: "电话", Type: 1}, {Name: "微信号", Type: 1}, {Name: "邮箱号", Type: 1},
		selectField("账号状态", "正常", "封禁", "已毕业"),
	}
}

type UserRow struct {
	ID         int             `db:"id"`
	Nickname   string          `db:"nickname"`
	MBTI       string          `db:"mbti"`
	School     string          `db:"school"`
	Major      string          `db:"major"`
	Grade      string          `db:"grade"`
	Intro      string          `db:"intro"`
	Experience string          `db:"experience"`
	Score      sql.NullFloat64 `db:"score"`
	Auth       int             `db:"auth"`
	Talent     int             `db:"talent"`
	Phone      string          `db:"phone"`
	Wechat     string          `db:"wechat"`
	Email      string          `db:"email"`
	Status     int             `db:"status"`
}

func (r UserRow) Values() (map[string]any, error) {
	values := []any{r.Nickname, r.MBTI, r.School, r.Major, r.Grade, r.Intro, r.Experience, nil, nil, "否", "未入驻人才库", r.Phone, r.Wechat, r.Email, "正常"}
	if r.Score.Valid {
		if math.IsNaN(r.Score.Float64) || math.IsInf(r.Score.Float64, 0) {
			return nil, fmt.Errorf("协作分数无效，用户 %d", r.ID)
		}
		values[7] = models.CollaborationLevel(r.Score.Float64)
		// The old CSV displayed two decimals, including rounding.
		values[8], _ = strconv.ParseFloat(strconv.FormatFloat(r.Score.Float64, 'f', 2, 64), 64)
	}
	if r.Auth == 1 {
		values[9] = "是"
	}
	if r.Talent == 1 {
		values[10] = "已入驻人才库"
	}
	if r.Status == 1 {
		values[14] = "封禁"
	} else if r.Status == 2 {
		values[14] = "已毕业"
	}
	result := make(map[string]any, 15)
	for i, field := range Fields() {
		value := values[i]
		if text, ok := value.(string); ok {
			if text == "" {
				value = nil
			} // Explicitly clear previous remote contents.
			if len([]rune(text)) > 100000 {
				return nil, fmt.Errorf("字段 %s 内容过长，用户 %d", field.Name, r.ID)
			}
			if field.Type == 3 && text != "" {
				valid := false
				for _, option := range field.Property.Options {
					if text == option.Name {
						valid = true
					}
				}
				if !valid {
					return nil, fmt.Errorf("字段 %s 存在未配置枚举，用户 %d", field.Name, r.ID)
				}
			}
		}
		result[field.Name] = value
	}
	return result, nil
}

// Validate instead of silently converting a field and potentially erasing data.
func ValidateFields(actual []Field) error {
	expected := Fields()
	if len(actual) != len(expected) {
		return fmt.Errorf("飞书表格必须严格保留原 15 列，请恢复表格结构")
	}
	for i, field := range expected {
		got := actual[i]
		if got.Name != field.Name || got.Type != field.Type || got.Hidden {
			return fmt.Errorf("飞书第 %d 列的名称、顺序或类型已变更，请恢复：%s", i+1, field.Name)
		}
		if field.Type == 2 && (got.Property == nil || got.Property.Formatter != "0.00") {
			return fmt.Errorf("请将协作具体分数值的格式恢复为两位小数")
		}
		if field.Type == 3 {
			for _, option := range field.Property.Options {
				found := false
				if got.Property != nil {
					for _, existing := range got.Property.Options {
						if option.Name == existing.Name {
							found = true
						}
					}
				}
				if !found {
					return fmt.Errorf("飞书单选字段 %s 缺少标签 %s，请恢复", field.Name, option.Name)
				}
			}
		}
	}
	return nil
}
