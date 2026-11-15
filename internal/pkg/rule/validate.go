package rule

import (
	"fmt"
	"reflect"
)

// ValidateOptionValues 拒绝不可哈希的表单选项值，必须在模板写入期调用。
// 渲染侧用选项值直接作 map[interface{}]string 的键（见 FieldProcessor.initialize），
// JSON 解出的数组与对象不可哈希，落库后会让通知协程 panic；
// 且 initialize 先于 provide 分支执行，企微工单同样会被连带崩掉。
func ValidateOptionValues(rules []Rule) error {
	for _, r := range rules {
		for _, opt := range r.Options {
			// nil 接口本身可作键，但 reflect.TypeOf(nil) 返回 nil，需先放行
			if opt.Value == nil {
				continue
			}
			if !reflect.TypeOf(opt.Value).Comparable() {
				return fmt.Errorf("字段 %s 的选项 %q 取值类型为 %T，不能作为选项值", r.Field, opt.Label, opt.Value)
			}
		}
	}
	return nil
}
