@echo off
setlocal

REM Canonical first selective backup: audience_profiles only.
REM Authentication must come from PGPASSWORD in the caller environment or pgpass.conf.
set "PGUSER=postgres"
set "PGDB=jazebeh"
set "PGHOST=127.0.0.1"
set "PGPORT=5432"
set "PGBIN=C:\Program Files\PostgreSQL\17\bin"
set "OUTDIR=E:\DB Backup"

if not exist "%PGBIN%\pg_dump.exe" (
    echo PostgreSQL 17 pg_dump was not found: "%PGBIN%\pg_dump.exe"
    exit /b 1
)
if not exist "%OUTDIR%" mkdir "%OUTDIR%"
if errorlevel 1 exit /b 1

for /f %%I in ('powershell.exe -NoProfile -Command "Get-Date -Format yyyyMMdd_HHmmss_fff"') do set "TS=%%I"
if not defined TS (
    echo Could not generate a locale-independent backup timestamp.
    exit /b 1
)
set "DUMP_FILE=%OUTDIR%\audience_profiles_%TS%.dump"
if exist "%DUMP_FILE%" (
    echo Refusing to overwrite existing backup: "%DUMP_FILE%"
    exit /b 1
)

"%PGBIN%\pg_dump.exe" -h "%PGHOST%" -p "%PGPORT%" -U "%PGUSER%" -d "%PGDB%" ^
  --format=plain ^
  --data-only ^
  --no-owner ^
  --no-privileges ^
  --strict-names ^
  -t public.audience_profiles ^
  --file="%DUMP_FILE%"

if errorlevel 1 (
    echo Backup failed.
    del /q "%DUMP_FILE%" 2>nul
    exit /b 1
)
for %%A in ("%DUMP_FILE%") do if %%~zA LEQ 0 (
    echo Backup output is empty.
    del /q "%DUMP_FILE%" 2>nul
    exit /b 1
)

echo Backup finished: "%DUMP_FILE%"
endlocal
