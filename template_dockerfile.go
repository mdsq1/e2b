package e2b

import (
	"fmt"
	"os"
	"strings"
)

// FromDockerfile parses Dockerfile content string and populates the template
// builder with equivalent instructions. Returns the builder for chaining.
//
// Supports: FROM, RUN, COPY, ENV, WORKDIR, USER, ENTRYPOINT, CMD.
// This is a simplified parser; it does not handle multi-stage builds,
// ARG substitution, or multi-line continuation characters.
func (t *TemplateBuilder) FromDockerfile(content string) *TemplateBuilder {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "FROM "):
			image := strings.TrimSpace(line[5:])
			// Strip AS alias for multi-stage (take only image part)
			if idx := strings.Index(strings.ToUpper(image), " AS "); idx >= 0 {
				image = strings.TrimSpace(image[:idx])
			}
			t.FromImage(image)
		case strings.HasPrefix(upper, "RUN "):
			cmd := strings.TrimSpace(line[4:])
			t.RunCmd(cmd)
		case strings.HasPrefix(upper, "COPY "):
			parts := parseDockerfileCopyArgs(line[5:])
			if len(parts) >= 2 {
				t.Copy(parts[len(parts)-2], parts[len(parts)-1])
			}
		case strings.HasPrefix(upper, "ENV "):
			envPart := strings.TrimSpace(line[4:])
			envs := parseEnvLine(envPart)
			if len(envs) > 0 {
				t.SetEnvs(envs)
			}
		case strings.HasPrefix(upper, "WORKDIR "):
			t.SetWorkdir(strings.TrimSpace(line[8:]))
		case strings.HasPrefix(upper, "USER "):
			t.SetUser(strings.TrimSpace(line[5:]))
		case strings.HasPrefix(upper, "ENTRYPOINT "):
			t.startCmd = strings.TrimSpace(line[11:])
		case strings.HasPrefix(upper, "CMD "):
			// CMD is used as start command if no ENTRYPOINT
			if t.startCmd == "" {
				t.startCmd = strings.TrimSpace(line[4:])
			}
		}
	}
	return t
}

// FromDockerfilePath reads a Dockerfile from the given path and parses it.
// Returns the builder for chaining.
func (t *TemplateBuilder) FromDockerfilePath(path string) *TemplateBuilder {
	data, err := os.ReadFile(path)
	if err != nil {
		if t.err == nil {
			t.err = fmt.Errorf("reading Dockerfile %q: %w", path, err)
		}
		return t
	}
	return t.FromDockerfile(string(data))
}

// parseDockerfileCopyArgs parses COPY arguments, stripping --chown/--chmod flags.
func parseDockerfileCopyArgs(args string) []string {
	fields := strings.Fields(strings.TrimSpace(args))
	var result []string
	for _, f := range fields {
		if strings.HasPrefix(f, "--") {
			continue // skip flags like --chown=user:group
		}
		result = append(result, f)
	}
	return result
}

// parseEnvLine parses ENV line format: KEY=VALUE or KEY VALUE.
// Handles quoted values like KEY="hello world" and KEY='value'.
func parseEnvLine(line string) map[string]string {
	envs := map[string]string{}
	if !strings.Contains(line, "=") {
		// Legacy single-pair format: KEY VALUE
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			envs[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
		return envs
	}
	// KEY=VALUE KEY2="quoted value" format — parse character by character
	remaining := strings.TrimSpace(line)
	for remaining != "" {
		eqIdx := strings.Index(remaining, "=")
		if eqIdx < 0 {
			break
		}
		key := strings.TrimSpace(remaining[:eqIdx])
		remaining = remaining[eqIdx+1:]

		var val string
		if len(remaining) > 0 && (remaining[0] == '"' || remaining[0] == '\'') {
			// Quoted value — find matching close quote
			quote := remaining[0]
			endIdx := strings.IndexByte(remaining[1:], quote)
			if endIdx >= 0 {
				val = remaining[1 : endIdx+1]
				remaining = remaining[endIdx+2:]
			} else {
				// No closing quote, take rest
				val = remaining[1:]
				remaining = ""
			}
		} else {
			// Unquoted value — read until next space
			spIdx := strings.IndexByte(remaining, ' ')
			if spIdx >= 0 {
				val = remaining[:spIdx]
				remaining = remaining[spIdx+1:]
			} else {
				val = remaining
				remaining = ""
			}
		}
		if key != "" {
			envs[key] = val
		}
		remaining = strings.TrimSpace(remaining)
	}
	return envs
}
