#pragma once

class IOnnxEnvironment {
public:
    virtual ~IOnnxEnvironment() = default;
    virtual void* getNativeEnv() = 0;
};
