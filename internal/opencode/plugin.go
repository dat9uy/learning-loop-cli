package opencode

// PluginDir is the project-local OpenCode plugin directory.
const PluginDir = ".opencode/plugins"

// PluginFileName is the only OpenCode artifact owned by this Installer.
const PluginFileName = "learning-loop.js"

// currentPluginSource is intentionally small: it invokes the raw renderer and
// appends its successful Markdown to OpenCode's native system context. Rule
// selection, parsing, and learning-loop settings remain in Go.
const currentPluginSource = `import { execFile } from "node:child_process"
import { promisify } from "node:util"

const execFileAsync = promisify(execFile)
const processFailureCode = "E208"

function asText(value) {
  if (value === undefined || value === null) return ""
  return typeof value === "string" ? value : String(value)
}

function diagnostic(error) {
  const details = [asText(error?.stderr), asText(error?.message)].filter(Boolean).join("\n")
  const match = details.match(/\bE\d{3}\b/)
  const code = match ? match[0] : processFailureCode
  const message = details.replace(/\s+/g, " ").trim() || "raw renderer process failed"
  return { code, message }
}

async function report(client, error) {
  const { code, message } = diagnostic(error)
  if (!client?.app?.log) return
  try {
    await client.app.log({
      body: {
        service: "learning-loop",
        level: "error",
        message: "learning-loop: opencode-adapter: " + code + ": " + message,
      },
    })
  } catch {
    // Diagnostics must never prevent OpenCode from continuing.
  }
}

export const LearningLoop = async ({ client, worktree }) => ({
  "experimental.chat.system.transform": async (_input, output) => {
    try {
      const result = await execFileAsync("learning-loop", ["render", worktree], { encoding: "utf8" })
      if (result.stdout) output.system.push(result.stdout)
    } catch (error) {
      await report(client, error)
    }
  },
})
`

// olderPluginSource is the exact shape recognized for controlled upgrades.
// It is retained here as a compatibility boundary rather than inferred from
// arbitrary JavaScript containing the word learning-loop.
const olderPluginSource = `import { execFile } from "node:child_process"
import { promisify } from "node:util"

const execFileAsync = promisify(execFile)

export const LearningLoop = async ({ worktree }) => ({
  "experimental.chat.system.transform": async (_input, output) => {
    const result = await execFileAsync("learning-loop", ["render", worktree], { encoding: "utf8" })
    if (result.stdout) output.system.push(result.stdout)
  },
})
`
