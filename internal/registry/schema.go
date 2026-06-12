package registry

type Param struct {
	Name        string `json:"name"`
	Flag        string `json:"flag"`
	Location    string `json:"location"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
	Max         int    `json:"max,omitempty"`
}

type Pagination struct {
	Type       string `json:"type"`
	MaxRows    int    `json:"maxRows,omitempty"`
	TotalField string `json:"totalField,omitempty"`
}

type Method struct {
	Name                  string     `json:"name"`
	CommandName           string     `json:"commandName"`
	HTTPMethod            string     `json:"httpMethod"`
	FullPath              string     `json:"fullPath"`
	Summary               string     `json:"summary"`
	Description           string     `json:"description,omitempty"`
	Params                []Param    `json:"params,omitempty"`
	Pagination            Pagination `json:"pagination"`
	RiskLevel             string     `json:"riskLevel"`
	RequiresOpenChannelID bool       `json:"requiresOpenChannelId"`
	TableColumns          []string   `json:"tableColumns,omitempty"`
}

type Service struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Domain      string   `json:"domain"`
	Methods     []Method `json:"methods"`
}

type Registry struct {
	Version  string    `json:"version"`
	Source   string    `json:"source"`
	Services []Service `json:"services"`
}

func (r Registry) AllMethods() []Method {
	var out []Method
	for _, svc := range r.Services {
		out = append(out, svc.Methods...)
	}
	return out
}

func (r Registry) Find(ref string) (Method, bool) {
	for _, svc := range r.Services {
		for _, method := range svc.Methods {
			if svc.Domain+"."+method.CommandName == ref || method.Name == ref || method.FullPath == ref {
				return method, true
			}
		}
	}
	return Method{}, false
}
