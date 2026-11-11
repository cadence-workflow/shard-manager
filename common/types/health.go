package types

// HealthStatus is an internal type (TBD...)
type HealthStatus struct {
	Ok  bool   `json:"ok,required"`
	Msg string `json:"msg,omitempty"`
}
