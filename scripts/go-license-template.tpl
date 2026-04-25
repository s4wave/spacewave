[
{{- $first := true -}}
{{- range . -}}
{{- if not $first }},{{ end -}}
{{- $first = false }}
  {
    "name": "{{ .Name }}",
    "version": "{{ .Version }}",
    "licenseName": "{{ .LicenseName }}",
    "licenseURL": "{{ .LicenseURL }}",
    "licenseText": {{ .LicenseText | printf "%q" }}
  }
{{- end }}
]
