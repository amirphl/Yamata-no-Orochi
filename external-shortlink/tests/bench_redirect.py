"""Small smoke-load generator: python tests/bench_redirect.py URL [concurrency] [requests]."""

from __future__ import annotations

import asyncio
import statistics
import sys
import time

import httpx


async def main(url: str, concurrency: int, requests: int) -> None:
    latencies: list[float] = []
    semaphore = asyncio.Semaphore(concurrency)
    async with httpx.AsyncClient(follow_redirects=False, timeout=5) as client:
        async def one() -> None:
            async with semaphore:
                started = time.perf_counter()
                response = await client.get(url)
                latencies.append((time.perf_counter() - started) * 1000)
                if response.status_code != 302:
                    raise RuntimeError(f"unexpected status {response.status_code}")

        started = time.perf_counter()
        await asyncio.gather(*(one() for _ in range(requests)))
        elapsed = time.perf_counter() - started
    ordered = sorted(latencies)
    percentile = lambda p: ordered[min(len(ordered) - 1, int(len(ordered) * p))]
    print(
        f"requests={requests} concurrency={concurrency} rps={requests / elapsed:.1f} "
        f"mean_ms={statistics.mean(latencies):.2f} p50_ms={percentile(.50):.2f} "
        f"p95_ms={percentile(.95):.2f} p99_ms={percentile(.99):.2f}"
    )


if __name__ == "__main__":
    asyncio.run(main(sys.argv[1], int(sys.argv[2]) if len(sys.argv) > 2 else 50, int(sys.argv[3]) if len(sys.argv) > 3 else 1000))
