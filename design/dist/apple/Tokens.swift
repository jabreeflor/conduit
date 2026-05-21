// Conduit Design Tokens — generated 2026-05-21. Do not edit by hand.
// Source: design/tokens.yaml. Regenerate with `make tokens`.

import SwiftUI

public enum ConduitTheme: String, CaseIterable, Sendable {
    case dark
    case light
    case hc
}

public extension Color {
    /// Look up a semantic Conduit color by its dotted path (e.g. "surface.canvas").
    static func conduit(_ path: String, theme: ConduitTheme = .dark) -> Color {
        switch theme {
        case .dark: return Self._conduit_dark[path] ?? .clear
        case .light: return Self._conduit_light[path] ?? .clear
        case .hc: return Self._conduit_hc[path] ?? .clear
        }
    }
}

private extension Color {
    static let _conduit_dark: [String: Color] = [
        "accent.cool": Color(red: 0.2118, green: 0.7020, blue: 0.8118),
        "accent.warm-alt": Color(red: 0.5569, green: 0.4275, blue: 0.8196),
        "agent.active": Color(red: 0.8627, green: 0.7686, blue: 0.5922),
        "agent.error": Color(red: 0.9529, green: 0.5098, blue: 0.5098),
        "agent.idle": Color(red: 0.6392, green: 0.6118, blue: 0.5725),
        "agent.thinking": Color(red: 0.2118, green: 0.7020, blue: 0.8118),
        "border.default": Color(red: 0.2941, green: 0.2745, blue: 0.2471),
        "border.focus": Color(red: 0.8627, green: 0.7686, blue: 0.5922),
        "border.strong": Color(red: 0.4863, green: 0.4627, blue: 0.4314),
        "border.subtle": Color(red: 0.1882, green: 0.1922, blue: 0.1804),
        "brand.on-primary": Color(red: 0.1059, green: 0.1098, blue: 0.0980),
        "brand.primary": Color(red: 0.9843, green: 0.9765, blue: 0.9569),
        "brand.primary-active": Color(red: 0.9176, green: 0.9098, blue: 0.8902),
        "brand.primary-hover": Color(red: 0.9608, green: 0.9529, blue: 0.9333),
        "model-badge.claude.bg": Color(red: 0.4039, green: 0.2196, blue: 0.0078),
        "model-badge.claude.border": Color(red: 0.5529, green: 0.3020, blue: 0.0078),
        "model-badge.claude.fg": Color(red: 1.0000, green: 0.8588, blue: 0.6118),
        "model-badge.litellm.bg": Color(red: 0.2549, green: 0.1412, blue: 0.0039),
        "model-badge.litellm.border": Color(red: 0.4039, green: 0.2196, blue: 0.0078),
        "model-badge.litellm.fg": Color(red: 1.0000, green: 0.8588, blue: 0.6118),
        "model-badge.local.bg": Color(red: 0.1020, green: 0.1137, blue: 0.1725),
        "model-badge.local.border": Color(red: 0.2431, green: 0.2627, blue: 0.3451),
        "model-badge.local.fg": Color(red: 0.8863, green: 0.8980, blue: 0.9333),
        "model-badge.openai.bg": Color(red: 0.0235, green: 0.2549, blue: 0.3098),
        "model-badge.openai.border": Color(red: 0.0392, green: 0.3569, blue: 0.4314),
        "model-badge.openai.fg": Color(red: 0.7373, green: 0.9020, blue: 0.9529),
        "model-badge.openrouter.bg": Color(red: 0.1725, green: 0.1137, blue: 0.3294),
        "model-badge.openrouter.border": Color(red: 0.2431, green: 0.1647, blue: 0.4627),
        "model-badge.openrouter.fg": Color(red: 0.8627, green: 0.8039, blue: 0.9490),
        "secondary.base": Color(red: 0.8627, green: 0.7686, blue: 0.5922),
        "secondary.container": Color(red: 0.3333, green: 0.2667, blue: 0.1333),
        "secondary.on": Color(red: 0.1059, green: 0.1098, blue: 0.0980),
        "secondary.on-container": Color(red: 0.9765, green: 0.8745, blue: 0.6941),
        "status.error": Color(red: 0.9529, green: 0.5098, blue: 0.5098),
        "status.info": Color(red: 0.2118, green: 0.7020, blue: 0.8118),
        "status.success": Color(red: 0.4353, green: 0.7961, blue: 0.5216),
        "status.warning": Color(red: 0.9608, green: 0.6196, blue: 0.1216),
        "surface.canvas": Color(red: 0.0706, green: 0.0745, blue: 0.0627),
        "surface.container": Color(red: 0.1882, green: 0.1922, blue: 0.1804),
        "surface.container-high": Color(red: 0.2941, green: 0.2745, blue: 0.2471),
        "surface.container-lowest": Color(red: 0.0706, green: 0.0745, blue: 0.0627),
        "surface.elevated": Color(red: 0.1882, green: 0.1922, blue: 0.1804),
        "surface.primary": Color(red: 0.1059, green: 0.1098, blue: 0.0980),
        "surface.sunken": Color(red: 0.0706, green: 0.0745, blue: 0.0627),
        "text.body": Color(red: 0.9843, green: 0.9765, blue: 0.9569),
        "text.inverted": Color(red: 0.1059, green: 0.1098, blue: 0.0980),
        "text.link": Color(red: 0.8627, green: 0.7686, blue: 0.5922),
        "text.muted": Color(red: 0.8039, green: 0.7725, blue: 0.7373),
        "text.subtle": Color(red: 0.6392, green: 0.6118, blue: 0.5725),
    ]
}

private extension Color {
    static let _conduit_light: [String: Color] = [
        "accent.cool": Color(red: 0.5490, green: 0.4706, blue: 0.3176),
        "accent.warm-alt": Color(red: 0.3216, green: 0.2196, blue: 0.5961),
        "agent.active": Color(red: 0.4314, green: 0.3608, blue: 0.2157),
        "agent.error": Color(red: 0.8627, green: 0.1843, blue: 0.1843),
        "agent.idle": Color(red: 0.6392, green: 0.6118, blue: 0.5725),
        "agent.thinking": Color(red: 0.0392, green: 0.3569, blue: 0.4314),
        "border.default": Color(red: 0.8039, green: 0.7725, blue: 0.7373),
        "border.focus": Color(red: 0.4314, green: 0.3608, blue: 0.2157),
        "border.strong": Color(red: 0.4863, green: 0.4627, blue: 0.4314),
        "border.subtle": Color(red: 0.8039, green: 0.7725, blue: 0.7373),
        "brand.on-primary": Color(red: 1.0000, green: 1.0000, blue: 1.0000),
        "brand.primary": Color(red: 0.1059, green: 0.1098, blue: 0.0980),
        "brand.primary-active": Color(red: 0.0706, green: 0.0745, blue: 0.0627),
        "brand.primary-hover": Color(red: 0.1882, green: 0.1922, blue: 0.1804),
        "model-badge.claude.bg": Color(red: 1.0000, green: 0.8588, blue: 0.6118),
        "model-badge.claude.border": Color(red: 0.9608, green: 0.6196, blue: 0.1216),
        "model-badge.claude.fg": Color(red: 0.4039, green: 0.2196, blue: 0.0078),
        "model-badge.litellm.bg": Color(red: 1.0000, green: 0.9529, blue: 0.8745),
        "model-badge.litellm.border": Color(red: 1.0000, green: 0.7451, blue: 0.3294),
        "model-badge.litellm.fg": Color(red: 0.4039, green: 0.2196, blue: 0.0078),
        "model-badge.local.bg": Color(red: 0.9176, green: 0.9098, blue: 0.8902),
        "model-badge.local.border": Color(red: 0.8039, green: 0.7725, blue: 0.7373),
        "model-badge.local.fg": Color(red: 0.2941, green: 0.2745, blue: 0.2471),
        "model-badge.openai.bg": Color(red: 0.7373, green: 0.9020, blue: 0.9529),
        "model-badge.openai.border": Color(red: 0.2118, green: 0.7020, blue: 0.8118),
        "model-badge.openai.fg": Color(red: 0.0235, green: 0.2549, blue: 0.3098),
        "model-badge.openrouter.bg": Color(red: 0.8627, green: 0.8039, blue: 0.9490),
        "model-badge.openrouter.border": Color(red: 0.5569, green: 0.4275, blue: 0.8196),
        "model-badge.openrouter.fg": Color(red: 0.1725, green: 0.1137, blue: 0.3294),
        "secondary.base": Color(red: 0.4314, green: 0.3608, blue: 0.2157),
        "secondary.container": Color(red: 0.9647, green: 0.8667, blue: 0.6824),
        "secondary.on": Color(red: 1.0000, green: 1.0000, blue: 1.0000),
        "secondary.on-container": Color(red: 0.4510, green: 0.3765, blue: 0.2314),
        "status.error": Color(red: 0.8627, green: 0.1843, blue: 0.1843),
        "status.info": Color(red: 0.0392, green: 0.3569, blue: 0.4314),
        "status.success": Color(red: 0.1647, green: 0.6157, blue: 0.2863),
        "status.warning": Color(red: 0.4314, green: 0.3608, blue: 0.2157),
        "surface.canvas": Color(red: 0.9843, green: 0.9765, blue: 0.9569),
        "surface.container": Color(red: 0.9412, green: 0.9333, blue: 0.9137),
        "surface.container-high": Color(red: 0.9176, green: 0.9098, blue: 0.8902),
        "surface.container-lowest": Color(red: 1.0000, green: 1.0000, blue: 1.0000),
        "surface.elevated": Color(red: 1.0000, green: 1.0000, blue: 1.0000),
        "surface.primary": Color(red: 1.0000, green: 1.0000, blue: 1.0000),
        "surface.sunken": Color(red: 0.9608, green: 0.9529, blue: 0.9333),
        "text.body": Color(red: 0.1059, green: 0.1098, blue: 0.0980),
        "text.inverted": Color(red: 0.9843, green: 0.9765, blue: 0.9569),
        "text.link": Color(red: 0.4314, green: 0.3608, blue: 0.2157),
        "text.muted": Color(red: 0.2941, green: 0.2745, blue: 0.2471),
        "text.subtle": Color(red: 0.4863, green: 0.4627, blue: 0.4314),
    ]
}

private extension Color {
    static let _conduit_hc: [String: Color] = [
        "accent.cool": Color(red: 0.4549, green: 0.8118, blue: 0.8980),
        "accent.warm-alt": Color(red: 0.7137, green: 0.6078, blue: 0.8941),
        "agent.active": Color(red: 1.0000, green: 0.7451, blue: 0.3294),
        "agent.error": Color(red: 0.9529, green: 0.5098, blue: 0.5098),
        "agent.idle": Color(red: 0.7529, green: 0.7725, blue: 0.8392),
        "agent.thinking": Color(red: 0.4549, green: 0.8118, blue: 0.8980),
        "border.default": Color(red: 0.8863, green: 0.8980, blue: 0.9333),
        "border.focus": Color(red: 1.0000, green: 0.7451, blue: 0.3294),
        "border.strong": Color(red: 1.0000, green: 1.0000, blue: 1.0000),
        "border.subtle": Color(red: 0.5490, green: 0.5765, blue: 0.6706),
        "brand.on-primary": Color(red: 0.0000, green: 0.0000, blue: 0.0000),
        "brand.primary": Color(red: 1.0000, green: 0.7451, blue: 0.3294),
        "brand.primary-active": Color(red: 0.9608, green: 0.6196, blue: 0.1216),
        "brand.primary-hover": Color(red: 1.0000, green: 0.8588, blue: 0.6118),
        "model-badge.claude.bg": Color(red: 0.0000, green: 0.0000, blue: 0.0000),
        "model-badge.claude.border": Color(red: 1.0000, green: 0.7451, blue: 0.3294),
        "model-badge.claude.fg": Color(red: 1.0000, green: 0.7451, blue: 0.3294),
        "model-badge.litellm.bg": Color(red: 0.0000, green: 0.0000, blue: 0.0000),
        "model-badge.litellm.border": Color(red: 1.0000, green: 0.8588, blue: 0.6118),
        "model-badge.litellm.fg": Color(red: 1.0000, green: 0.8588, blue: 0.6118),
        "model-badge.local.bg": Color(red: 0.0000, green: 0.0000, blue: 0.0000),
        "model-badge.local.border": Color(red: 1.0000, green: 1.0000, blue: 1.0000),
        "model-badge.local.fg": Color(red: 1.0000, green: 1.0000, blue: 1.0000),
        "model-badge.openai.bg": Color(red: 0.0000, green: 0.0000, blue: 0.0000),
        "model-badge.openai.border": Color(red: 0.4549, green: 0.8118, blue: 0.8980),
        "model-badge.openai.fg": Color(red: 0.4549, green: 0.8118, blue: 0.8980),
        "model-badge.openrouter.bg": Color(red: 0.0000, green: 0.0000, blue: 0.0000),
        "model-badge.openrouter.border": Color(red: 0.7137, green: 0.6078, blue: 0.8941),
        "model-badge.openrouter.fg": Color(red: 0.7137, green: 0.6078, blue: 0.8941),
        "secondary.base": Color(red: 0.8627, green: 0.7686, blue: 0.5922),
        "secondary.container": Color(red: 0.0000, green: 0.0000, blue: 0.0000),
        "secondary.on": Color(red: 0.0000, green: 0.0000, blue: 0.0000),
        "secondary.on-container": Color(red: 0.8627, green: 0.7686, blue: 0.5922),
        "status.error": Color(red: 0.9529, green: 0.5098, blue: 0.5098),
        "status.info": Color(red: 0.4549, green: 0.8118, blue: 0.8980),
        "status.success": Color(red: 0.4353, green: 0.7961, blue: 0.5216),
        "status.warning": Color(red: 1.0000, green: 0.7451, blue: 0.3294),
        "surface.canvas": Color(red: 0.0000, green: 0.0000, blue: 0.0000),
        "surface.container": Color(red: 0.0392, green: 0.0471, blue: 0.0784),
        "surface.container-high": Color(red: 0.0627, green: 0.0745, blue: 0.1176),
        "surface.container-lowest": Color(red: 0.0000, green: 0.0000, blue: 0.0000),
        "surface.elevated": Color(red: 0.0392, green: 0.0471, blue: 0.0784),
        "surface.primary": Color(red: 0.0000, green: 0.0000, blue: 0.0000),
        "surface.sunken": Color(red: 0.0000, green: 0.0000, blue: 0.0000),
        "text.body": Color(red: 1.0000, green: 1.0000, blue: 1.0000),
        "text.inverted": Color(red: 0.0000, green: 0.0000, blue: 0.0000),
        "text.link": Color(red: 0.4549, green: 0.8118, blue: 0.8980),
        "text.muted": Color(red: 0.8863, green: 0.8980, blue: 0.9333),
        "text.subtle": Color(red: 0.7529, green: 0.7725, blue: 0.8392),
    ]
}

