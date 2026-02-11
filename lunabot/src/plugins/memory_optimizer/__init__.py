import platform
import ctypes
import os
import asyncio
from nonebot import require, logger, get_driver
from nonebot.plugin import PluginMetadata

#require("nonebot_plugin_apscheduler")
from nonebot_plugin_apscheduler import scheduler

__plugin_meta__ = PluginMetadata(
    name="内存优化",
    description="定期调用 malloc_trim 释放内存 (Linux Only)",
    usage="自动运行",
    type="application",
    homepage="",
    supported_adapters=None,
)

# 仅在 Linux 下生效
IS_LINUX = platform.system() == "Linux"

def get_rss_memory() -> int:
    """获取当前进程的 RSS 内存占用 (字节)"""
    try:
        import psutil
        process = psutil.Process(os.getpid())
        return process.memory_info().rss
    except ImportError:
        # 当 psutil 不可用时的回退方案
        try:
            with open(f"/proc/{os.getpid()}/statm", "r") as f:
                # statm: size resident share text lib data dt
                # resident 是页数，页大小通常为 4KB
                resident_pages = int(f.read().split()[1])
                return resident_pages * os.sysconf("SC_PAGE_SIZE")
        except Exception:
            return 0

def format_size(size_bytes: int) -> str:
    """格式化字节数为人类可读格式"""
    for unit in ["B", "KB", "MB", "GB"]:
        if size_bytes < 1024:
            return f"{size_bytes:.2f} {unit}"
        size_bytes /= 1024
    return f"{size_bytes:.2f} TB"

@scheduler.scheduled_job("interval", minutes=1, id="memory_trim_job")
async def memory_trim_job():
    if not IS_LINUX:
        return
    await perform_memory_trim(quiet=True)

async def perform_memory_trim(quiet: bool = False) -> str:
    """执行内存清理并返回结果信息"""
    msg = ""
    try:
        from ..sekai.modules.mysekai import harvest_point_image_offsets_cache
        from ..sekai.asset import RegionRipAssetManger

        # 1. 清理 mysekai 缓存
        if hasattr(harvest_point_image_offsets_cache, 'clear'):
            harvest_point_image_offsets_cache.clear()
        
        # 2. 清理 asset 缓存 (遍历所有管理器)
        cleared_assets = 0
        for mgr in RegionRipAssetManger._all_mgrs.values():
            if hasattr(mgr, 'cached_images'):
                cleared_assets += len(mgr.cached_images)
                mgr.cached_images.clear()

        if not quiet:
            msg += f"\n已清理 {cleared_assets} 张解包资源图片缓存"

        libc = ctypes.CDLL("libc.so.6")
        
        rss_before = get_rss_memory()
        
        # trim(0) 尽可能释放所有可释放的内存
        ret = libc.malloc_trim(0)
        
        rss_after = get_rss_memory()
        
        if ret:
            diff = rss_before - rss_after
            msg += (
                f"内存已清理。\n"
                f"清理前: {format_size(rss_before)}\n"
                f"清理后: {format_size(rss_after)}\n"
                f"释放: {format_size(diff)}"
            )
            
            # 自动模式下，只有释放超过 1MB 才打印日志
            if not quiet or diff > 1024 * 1024:
                logger.info(f"内存优化器: {msg.replace(chr(10), ', ')}") # 将换行符替换为逗号以便记录日志
        else:
            msg = "内存已清理。未释放内存。"
            if not quiet:
                logger.info("内存优化器: 未释放内存。")
            
    except Exception as e:
        msg = f"内存清理失败: {e}"
        logger.warning(f"内存优化器: {msg}")
    
    return msg

# 手动清理指令
# 手动清理指令
from ..utils import CmdHandler, HandlerContext

clean_memory = CmdHandler(["/清理内存", "/gc", "/内存清理"], logger)
clean_memory.check_superuser()

@clean_memory.handle()
async def _(ctx: HandlerContext):
    if not IS_LINUX:
        return await ctx.asend_reply_msg(f"内存优化插件仅支持 Linux (当前: {platform.system()})")

    await ctx.asend_reply_msg("正在执行内存清理...")
    result = await perform_memory_trim(quiet=False)
    # asend_reply_msg 会自动处理回复逻辑
    await ctx.asend_reply_msg(result)

driver = get_driver()

@driver.on_startup
async def _():
    if IS_LINUX:
        logger.info("内存优化器: 插件已加载。已调度每 1 分钟执行 malloc_trim。使用 /清理内存 手动触发。")
    else:
        logger.info(f"内存优化器: 未在 Linux 上运行 (当前: {platform.system()})，插件已禁用。")
