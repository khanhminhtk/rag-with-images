#pragma once

#include <memory>
#include <string>

#include <spdlog/spdlog.h>
#include <spdlog/sinks/rotating_file_sink.h>
#include <spdlog/sinks/stdout_color_sinks.h>

#include "application/port/ILogger.hpp"

class SpdLogger : public ILogger {
public:
    SpdLogger(const std::string& filePath, const std::string& logName) {
        auto console_sink = std::make_shared<spdlog::sinks::stdout_color_sink_mt>();
        auto file_sink    = std::make_shared<spdlog::sinks::rotating_file_sink_mt>(
            filePath, 1024 * 1024 * 5, 3);

        std::vector<spdlog::sink_ptr> sinks{console_sink, file_sink};
        logger_ = std::make_shared<spdlog::logger>(logName, sinks.begin(), sinks.end());
        logger_->set_pattern("[%Y-%m-%d %H:%M:%S.%e] [%n] [%^%l%$] %v");
        logger_->set_level(spdlog::level::debug);
    }

    void info(const std::string& msg) override    { logger_->info(msg); }
    void error(const std::string& msg) override   { logger_->error(msg); }
    void warning(const std::string& msg) override { logger_->warn(msg); }
    void debug(const std::string& msg) override   { logger_->debug(msg); }

private:
    std::shared_ptr<spdlog::logger> logger_;
};
