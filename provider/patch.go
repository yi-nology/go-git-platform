package provider

// ResolvePatch 是 Update*Options 三态解析核心（2A，spec
// 2026-11-12-phase2a-tristate-design）：
//
//   - patch 非 nil：全量替换——返回 *patch，specified=true；空值（"" /
//     空切片）即显式清空；
//   - patch nil：回落旧字段语义——isEmpty(legacy) 时 specified=false（不
//     动），否则返回 legacy、specified=true（置值）；
//   - 双字段同设：patch 立即返回（优先，spec 裁决 3）。
//
// specified=false → 后端不动该 wire 字段；specified && isEmpty(val) →
// 清空（平台 B 级则返回 ErrClearNotSupported）；specified && !isEmpty →
// 置值。
func ResolvePatch[T any](patch *T, legacy T, isEmpty func(T) bool) (val T, specified bool) {
	if patch != nil {
		return *patch, true
	}
	if isEmpty(legacy) {
		var zero T
		return zero, false
	}
	return legacy, true
}

// --- UpdateIssueOptions 三态解析（value receiver，只读）---

func (o UpdateIssueOptions) ResolveBody() (string, bool) {
	return ResolvePatch(o.BodyPatch, o.Body, func(s string) bool { return s == "" })
}

func (o UpdateIssueOptions) ResolveAssignees() ([]string, bool) {
	return ResolvePatch(o.AssigneesPatch, o.Assignees, func(s []string) bool { return len(s) == 0 })
}

func (o UpdateIssueOptions) ResolveLabels() ([]string, bool) {
	return ResolvePatch(o.LabelsPatch, o.Labels, func(s []string) bool { return len(s) == 0 })
}

func (o UpdateIssueOptions) ResolveMilestone() (string, bool) {
	return ResolvePatch(o.MilestonePatch, o.Milestone, func(s string) bool { return s == "" })
}

// --- UpdateCROptions 三态解析 ---

func (o UpdateCROptions) ResolveDescription() (string, bool) {
	return ResolvePatch(o.DescriptionPatch, o.Description, func(s string) bool { return s == "" })
}

func (o UpdateCROptions) ResolveAssignees() ([]string, bool) {
	return ResolvePatch(o.AssigneesPatch, o.Assignees, func(s []string) bool { return len(s) == 0 })
}
