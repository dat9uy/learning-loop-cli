package pi

import "strings"

// ExtensionDir is the project-local pi extension directory.
const ExtensionDir = ".pi/extensions"

// ExtensionFileName is the only pi artifact owned by this Installer.
const ExtensionFileName = "learning-loop.ts"

// projectRootPlaceholder marks where the selected project root is embedded.
// It is replaced exactly once when the extension source is generated.
const projectRootPlaceholder = "__LEARNING_LOOP_PROJECT_ROOT__"

// currentExtensionSource is intentionally small: it invokes the raw renderer
// and appends its successful Markdown to pi's chained system prompt before
// the agent loop starts. Rule selection, parsing, and learning-loop settings
// remain in Go. The only import is type-only, so the extension has no
// runtime dependencies.
const currentExtensionTemplate = `// learning-loop pi extension: delivers the selected project's Rules as
// Instructions by appending rendered Markdown to pi's chained system prompt
// before the agent loop starts. Rule selection and rendering live in the
// learning-loop CLI; this file only invokes the raw renderer and fails open.
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

const projectRoot = "__LEARNING_LOOP_PROJECT_ROOT__";

const processFailureCode = "E209";

function asText(value) {
  if (value === undefined || value === null) return "";
  return typeof value === "string" ? value : String(value);
}

function diagnostic(result) {
  const details = [asText(result?.stderr), asText(result?.message)].filter(Boolean).join("\n");
  const match = details.match(/\bE\d{3}\b/);
  const code = match ? match[0] : processFailureCode;
  const message = details.replace(/\s+/g, " ").trim() || "raw renderer process failed";
  return { code, message };
}

function report(ctx, result) {
  if (!ctx?.hasUI) return;
  const { code, message } = diagnostic(result);
  try {
    ctx.ui.notify("learning-loop: pi-adapter: " + code + ": " + message, "error");
  } catch {
    // Diagnostics must never prevent pi from continuing.
  }
}

export default function (pi: ExtensionAPI) {
  pi.on("before_agent_start", async (event, ctx) => {
    try {
      const result = await pi.exec("learning-loop", ["render", projectRoot]);
      if (result.code !== 0) {
        report(ctx, result);
        return;
      }
      if (result.stdout) {
        return { systemPrompt: event.systemPrompt + "\n\n" + result.stdout };
      }
    } catch (error) {
      report(ctx, error);
    }
  });
}
`

// currentExtensionSource returns the exact extension source for the selected
// project root. The root is embedded so delivery always renders the connected
// project regardless of where pi is started.
func currentExtensionSource(projectRoot string) string {
	return strings.Replace(currentExtensionTemplate, projectRootPlaceholder, projectRoot, 1)
}
