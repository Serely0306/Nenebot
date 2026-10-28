"""Command policy for CN captures and their shared static queries."""

CAPTURE_COMMANDS = frozenset({
    "/card",
    "/pjsk box",
    "/pjsk card material",
    "/cr\u4efb\u52a1",
    "/pjsk event card",
    "/pjsk challenge card",
    "/pjsk no event deck",
    "/pjsk bonus deck",
    "/mysekai deck",
    "/pjsk score up",
    "/pjsk challenge info",
    "/pjsk power bonus info",
    "/pjsk area item",
    "/pjsk bonds",
    "/pjskcr",
    "/pjsk leader count",
    "/pjsk material",
    "/pjsk event record",
    "/pjsk gacha record",
    "/\u6293\u5305\u5e2e\u52a9",
    "/pjskdetail",
    "/pjsk song list",
    "/pjsk progress",
    "/pjsk music rewards",
    "/anvo",
    "/pjsk b30",
    "/pjsk mysekai res",
    "/pjsk mysekai blueprint",
    "/pjsk mysekai furniture",
    "/pjsk mysekai photo",
    "/pjsk check mysekai data",
    "/pjsk mysekai gate",
    "/pjsk mysekai musicrecord",
    "/msr\u6362\u7ed1",
    "/pjsk mysekai\u6750\u6599",
    "/pjsk bind",
    "/pjsk unbind",
    "/pjsk set main",
    "/pjsk swap bind",
    "/pjsk hide suite",
    "/pjsk show suite",
    "/pjsk hide id",
    "/pjsk show id",
    "/pjsk data mode",
    "/pjsk check data",
    "/pjsk blacklist add",
    "/pjsk blacklist remove",
    "/pjsk verify list",
    "/pjsk user sta",
    "/pjsk bind history",
    "/pjsk_group_sub_msr",
    "/pjsk_group_unsub_msr",
    "/pjsk_sub_msr",
    "/pjsk_unsub_msr"
})

_UNSUPPORTED_MESSAGE = (
    "\u5f53\u524d\u4e0d\u652f\u6301\u529f\u80fd\uff0c"
    "\u8bf7\u66f4\u6362\u524d\u7f00\u4f7f\u7528HarukiBot NEO\u529f\u80fd"
)


def is_capture_command(commands: list[str]) -> bool:
    """Match a canonical alias before region and separator expansion."""
    return any(command in CAPTURE_COMMANDS for command in commands)


def make_unsupported_reply(
    original_text: str | None, fallback_command: str, args: str
) -> str:
    """Suggest the replacement prefix only when the original input is known."""
    text = original_text.lstrip() if original_text else ""
    if text.startswith("*"):
        replacement = "/" + text[1:]
    elif text.startswith("/"):
        replacement = text[1:]
    else:
        replacement = None

    if replacement is not None:
        return _UNSUPPORTED_MESSAGE + "\n\u8bf7\u6539\u7528\uff1a" + replacement

    reply = (
        _UNSUPPORTED_MESSAGE
        + "\uff08\u5f53\u524d\u662f * \u5219\u6539\u4e3a /\uff0c"
        "\u5f53\u524d\u662f / \u5219\u53bb\u6389 /\uff09"
    )
    command = fallback_command.strip()
    if command.startswith(("/", "*")):
        command = command[1:]
    if command:
        if args.strip():
            command += " " + args.strip()
        reply += "\n\u793a\u4f8b\uff1a/" + command + " \u6216 " + command
    return reply
