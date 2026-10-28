"""Lightweight replacement handlers for retired public-data features."""

from ..common import cd, gbl
from ..handler import SekaiCmdHandler, reject_unsupported_command

# Copied from the original command declarations without importing their modules.
UNSUPPORTED_COMMAND_SPECS = (
    {"source": "modules/chart.py", "name": "pjsk_chart", "commands": ["/pjsk chart", "/\u8c31\u9762\u67e5\u8be2", "/\u94fa\u9762\u67e5\u8be2", "/\u8c31\u9762\u9884\u89c8", "/\u94fa\u9762\u9884\u89c8", "/\u8c31\u9762", "/\u94fa\u9762", "/\u67e5\u8c31\u9762", "/\u67e5\u94fa\u9762", "/\u67e5\u8c31", "/\u6280\u80fd\u9884\u89c8"], "prefix_args": [""], "regions": ["jp", "cn"]},
    {"source": "modules/entertainment.py", "name": "pjsk_entertainment_limit", "commands": ["/pjsk entertainment limit", "/pjsk_entertainment_limit", "/pjsk\u5a31\u4e50\u529f\u80fd\u4e0a\u9650", "/pel"], "prefix_args": [""], "regions": ["jp"]},
    {"source": "modules/entertainment.py", "name": "pjsk_entertainment_limit_check", "commands": ["/pjsk entertainment count", "/pjsk_entertainment_count", "/pjsk\u5a31\u4e50\u529f\u80fd\u6b21\u6570", "/pec"], "prefix_args": [""], "regions": ["jp"]},
    {"source": "modules/entertainment.py", "name": "pjsk_guess_cover", "commands": ["/pjsk guess cover", "/pjsk_guess_cover", "/pjsk\u731c\u66f2\u5c01", "/pjsk\u731c\u66f2\u7ed8", "/\u731c\u66f2\u7ed8", "/\u731c\u66f2\u5c01"], "prefix_args": [""], "regions": ["jp"]},
    {"source": "modules/entertainment.py", "name": "pjsk_guess_chart", "commands": ["/pjsk guess chart", "/pjsk_guess_chart", "/pjsk\u731c\u8c31\u9762", "/\u731c\u8c31\u9762", "/pjsk\u731c\u94fa\u9762", "/\u731c\u94fa\u9762"], "prefix_args": [""], "regions": ["jp"]},
    {"source": "modules/entertainment.py", "name": "pjsk_guess_card", "commands": ["/pjsk guess card", "/pjsk_guess_card", "/pjsk\u731c\u5361\u9762", "/\u731c\u5361\u9762", "/pjsk\u731c\u5361", "/\u731c\u5361"], "prefix_args": [""], "regions": ["jp"]},
    {"source": "modules/entertainment.py", "name": "pjsk_guess_music", "commands": ["/pjsk guess music", "/pjsk_guess_music", "/\u542c\u6b4c\u8bc6\u66f2", "/pjsk\u542c\u6b4c\u8bc6\u66f2", "/\u731c\u6b4c", "/pjsk\u731c\u6b4c", "/\u731c\u66f2", "/pjsk\u731c\u66f2"], "prefix_args": [""], "regions": ["jp"]},
    {"source": "modules/entertainment.py", "name": "pjsk_spin_gacha", "commands": ["/\u5355\u62bd", "/1\u8fde", "/\u5341\u8fde", "/2\u8fde", "/3\u8fde", "/4\u8fde", "/5\u8fde", "/6\u8fde", "/7\u8fde", "/8\u8fde", "/9\u8fde", "/10\u8fde", "/20\u8fde", "/30\u8fde", "/40\u8fde", "/50\u8fde", "/60\u8fde", "/70\u8fde", "/80\u8fde", "/90\u8fde", "/100\u8fde", "/110\u8fde", "/120\u8fde", "/130\u8fde", "/140\u8fde", "/150\u8fde", "/160\u8fde", "/170\u8fde", "/180\u8fde", "/190\u8fde", "/200\u8fde", "/210\u8fde", "/220\u8fde", "/230\u8fde", "/240\u8fde", "/250\u8fde", "/260\u8fde", "/270\u8fde", "/280\u8fde", "/290\u8fde", "/300\u8fde", "/310\u8fde", "/320\u8fde", "/330\u8fde", "/340\u8fde", "/350\u8fde", "/360\u8fde", "/370\u8fde", "/380\u8fde", "/390\u8fde", "/400\u8fde"], "prefix_args": [""], "regions": ["jp", "cn"]},
    {"source": "modules/entertainment.py", "name": "pjsk_user_limit", "commands": ["/pjsk\u5a31\u4e50\u9650\u5236", "/pjsk user limit", "/pul"], "prefix_args": [""], "regions": ["jp"]},
    {"source": "modules/entertainment.py", "name": "pjsk_user_usage", "commands": ["/pjsk\u5a31\u4e50\u6b21\u6570", "/pjsk user usage", "/peu"], "prefix_args": [""], "regions": ["jp"]},
    {"source": "modules/entertainment.py", "name": "pjsk_entertainment_stats", "commands": ["/pjsk\u5a31\u4e50\u7edf\u8ba1", "/pjsk entertainment stats", "/pes"], "prefix_args": [""], "regions": ["jp"]},
    {"source": "modules/score.py", "name": "pjsk_score_control", "commands": ["/pjsk score", "/\u63a7\u5206"], "prefix_args": ["wl"], "regions": ["jp"]},
    {"source": "modules/score.py", "name": "pjsk_custom_room_score_control", "commands": ["/pjsk custom room score", "/custom room score", "/\u81ea\u5b9a\u4e49\u623f\u95f4\u63a7\u5206", "/\u81ea\u5b9a\u4e49\u623f\u63a7\u5206", "/\u81ea\u5b9a\u4e49\u63a7\u5206"], "prefix_args": [""], "regions": ["jp"]},
    {"source": "modules/score.py", "name": "pjsk_music_meta", "commands": ["/pjsk music meta", "/music meta", "/\u6b4c\u66f2meta"], "prefix_args": [""], "regions": ["jp"]},
    {"source": "modules/score.py", "name": "pjsk_music_board", "commands": ["/pjsk music board", "/music board", "/\u6b4c\u66f2\u6392\u884c", "/\u6b4c\u66f2\u6bd4\u8f83", "/\u6b4c\u66f2\u6392\u540d"], "prefix_args": [""], "regions": ["jp"]},
    {"source": "modules/sk.py", "name": "pjsk_skp", "commands": ["/pjsk sk predict", "/pjsk board predict", "/sk\u9884\u6d4b", "/\u699c\u7ebf\u9884\u6d4b", "/skp"], "prefix_args": ["", "wl"], "regions": ["jp", "cn"]},
    {"source": "modules/sk.py", "name": "pjsk_skl", "commands": ["/pjsk sk line", "/pjsk board line", "/sk\u7ebf", "/skl", "/\u699c\u7ebf"], "prefix_args": ["", "wl"], "regions": ["jp", "cn"]},
    {"source": "modules/sk.py", "name": "pjsk_sks", "commands": ["/pjsk sk speed", "/pjsk board speed", "/\u65f6\u901f", "/sks", "/skv", "/sk\u65f6\u901f"], "prefix_args": ["", "wl"], "regions": ["jp", "cn"]},
    {"source": "modules/sk.py", "name": "pjsk_skds", "commands": ["/pjsk sk daily speed", "/pjsk board daily speed", "/\u65e5\u901f", "/skds", "/skdv", "/sk\u65e5\u901f"], "prefix_args": ["", "wl"], "regions": ["jp", "cn"]},
    {"source": "modules/sk.py", "name": "pjsk_sk", "commands": ["/pjsk sk board", "/pjsk board", "/sk"], "prefix_args": ["", "wl"], "regions": ["jp", "cn"]},
    {"source": "modules/sk.py", "name": "pjsk_cf", "commands": ["/cf", "/\u67e5\u623f", "/pjsk\u67e5\u623f"], "prefix_args": ["", "wl"], "regions": ["jp", "cn"]},
    {"source": "modules/sk.py", "name": "pjsk_csb", "commands": ["/csb", "/\u67e5\u6c34\u8868", "/pjsk\u67e5\u6c34\u8868", "/\u505c\u8f66\u65f6\u95f4"], "prefix_args": ["", "wl"], "regions": ["jp", "cn"]},
    {"source": "modules/sk.py", "name": "pjsk_ptr", "commands": ["/ptr", "/\u73a9\u5bb6\u8ffd\u8e2a", "/pjsk\u73a9\u5bb6\u8ffd\u8e2a"], "prefix_args": ["", "wl"], "regions": ["jp", "cn"]},
    {"source": "modules/sk.py", "name": "pjsk_rtr", "commands": ["/rtr", "/skt", "/\u8ffd\u8e2a", "/pjsk\u8ffd\u8e2a", "/sklt", "/sktl", "/\u5206\u6570\u7ebf\u8ffd\u8e2a", "/pjsk\u5206\u6570\u7ebf\u8ffd\u8e2a"], "prefix_args": ["", "wl"], "regions": ["jp", "cn"]},
    {"source": "modules/sk.py", "name": "pjsk_winrate", "commands": ["/pjsk winrate predict", "/\u80dc\u7387\u9884\u6d4b", "/5v5\u9884\u6d4b", "/\u80dc\u7387", "/5v5\u80dc\u7387", "/\u9884\u6d4b\u80dc\u7387", "/\u9884\u6d4b5v5"], "prefix_args": [""], "regions": ["jp"]},
    {"source": "modules/stamp.py", "name": "pjsk_stamp", "commands": ["/pjsk stamp", "/pjsk bq", "/pjsk\u8868\u60c5", "/pjsk\u8868\u60c5\u5236\u4f5c"], "prefix_args": [""], "regions": ["jp", "cn"]},
    {"source": "modules/stamp.py", "name": "pjsk_rand_stamp", "commands": ["/pjsk rand stamp", "/pjsk rand bq", "/pjsk\u968f\u673a\u8868\u60c5", "/pjsk\u968f\u673a\u8868\u60c5\u5236\u4f5c", "/\u968f\u673a\u8868\u60c5"], "prefix_args": [""], "regions": ["jp", "cn"]},
    {"source": "modules/stamp.py", "name": "pjsk_stamp_refresh", "commands": ["/pjsk stamp refresh", "/pjsk refresh stamp", "/pjsk\u8868\u60c5\u5237\u65b0", "/pjsk\u5237\u65b0\u8868\u60c5", "/pjsk\u5237\u65b0\u8868\u60c5\u5e95\u56fe", "/pjsk\u8868\u60c5\u5237\u65b0\u5e95\u56fe"], "prefix_args": [""], "regions": ["jp", "cn"]},
    {"source": "modules/stamp.py", "name": "pjsk_stamp_refresh_batch", "commands": ["/pjsk stamp refresh batch", "/pjsk\u8868\u60c5\u5237\u65b0\u6279\u91cf"], "prefix_args": [""], "regions": ["jp", "cn"]},
    {"source": "modules/stamp.py", "name": "pjsk_stamp_base", "commands": ["/pjsk stamp base", "/pjsk\u8868\u60c5\u5e95\u56fe"], "prefix_args": [""], "regions": ["jp", "cn"]},
    {"source": "modules/stamp.py", "name": "pjsk_stamp_base_delete", "commands": ["/pjsk remove stamp base", "/pjsk del stamp base", "/pjsk\u5220\u9664\u8868\u60c5\u5e95\u56fe"], "prefix_args": [""], "regions": ["jp", "cn"]},
    {"source": "modules/vlive.py", "name": "pjsk_live", "commands": ["/pjsk live", "/\u865a\u62dflive", "/pjsk vlive", "/vlive"], "prefix_args": [""], "regions": ["jp", "cn"]},
    {"source": "modules/wish.py", "name": "pjsk_wishsk", "commands": ["/wishsk"], "prefix_args": [""], "regions": ["cn"]},
    {"source": "modules/wish.py", "name": "pjsk_wishskl", "commands": ["/wishsk\u7ebf"], "prefix_args": [""], "regions": ["cn"]},
    {"source": "modules/vlive.py", "name": "vlive_group_sub_subscribe", "commands": ["/pjsk_group_sub_live", "/pjsk_sub_group_live", "/pjsk group sub live", "/pjsk sub group live", "/pjsk\u5f00\u542f live", "/pjsk \u5f00\u542f live", "/pjsk\u5f00\u542flive", "/pjsk \u5f00\u542flive"], "prefix_args": [""], "regions": ["jp", "cn"]},
    {"source": "modules/vlive.py", "name": "vlive_group_sub_unsubscribe", "commands": ["/pjsk_group_unsub_live", "/pjsk_unsub_group_live", "/pjsk group unsub live", "/pjsk unsub group live", "/pjsk\u5173\u95ed live", "/pjsk \u5173\u95ed live", "/pjsk\u5173\u95edlive", "/pjsk \u5173\u95edlive"], "prefix_args": [""], "regions": ["jp", "cn"]},
    {"source": "modules/vlive.py", "name": "vlive_user_sub_subscribe", "commands": ["/pjsk_sub_live", "/pjsk sub live", "/pjsk\u8ba2\u9605 live", "/pjsk \u8ba2\u9605 live", "/pjsk\u8ba2\u9605live", "/pjsk \u8ba2\u9605live"], "prefix_args": [""], "regions": ["jp", "cn"]},
    {"source": "modules/vlive.py", "name": "vlive_user_sub_unsubscribe", "commands": ["/pjsk_unsub_live", "/pjsk unsub live", "/pjsk\u53d6\u6d88 live", "/pjsk \u53d6\u6d88 live", "/pjsk\u53d6\u6d88live", "/pjsk \u53d6\u6d88live", "/pjsk\u53d6\u6d88\u8ba2\u9605 live", "/pjsk \u53d6\u6d88\u8ba2\u9605 live", "/pjsk\u53d6\u6d88\u8ba2\u9605live", "/pjsk \u53d6\u6d88\u8ba2\u9605live"], "prefix_args": [""], "regions": ["jp", "cn"]},

)


def _register_unsupported(spec):
    handler = SekaiCmdHandler(
        spec["commands"],
        regions=spec["regions"],
        prefix_args=spec["prefix_args"],
        capture_only=False,
    )
    handler.check_cdrate(cd).check_wblist(gbl)

    @handler.handle()
    async def unsupported(ctx):
        return await reject_unsupported_command(ctx)

    return handler


UNSUPPORTED_HANDLERS = {
    spec["name"]: _register_unsupported(spec)
    for spec in UNSUPPORTED_COMMAND_SPECS
}

__all__ = ["UNSUPPORTED_HANDLERS"]
