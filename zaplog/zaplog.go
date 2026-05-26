package zaplog

import (
	"context"
	"os"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// TraceIDKey نوع اختصاصی برای جلوگیری از تداخل کلیدها در Context
type contextKey string

const TraceIDKey contextKey = "trace_id"

// Config تنظیمات اولیه لاگر را نگه می‌دارد
type Config struct {
	Level    string // "debug", "info", "warn", "error"
	Encoding string // "json" یا "console"
	Output   string // "stdout" یا "file"
	FilePath string // مسیر و نام پایه فایل (مثلا: "logs/app")
}

// Logger اینترفیسی که تمام سرویس‌ها و هندلرهای شما به آن وابسته‌اند
type Logger interface {
	Debug(ctx context.Context, msg string, fields ...zap.Field)
	Info(ctx context.Context, msg string, fields ...zap.Field)
	Warn(ctx context.Context, msg string, fields ...zap.Field)
	Error(ctx context.Context, msg string, fields ...zap.Field)
	Fatal(ctx context.Context, msg string, fields ...zap.Field)
	Sync() error
}

type zapLogger struct {
	logger *zap.Logger
}

var loggerForErrorGeneral Logger

// NewZapLogger یک لاگر جدید بر اساس تنظیمات می‌سازد
func NewZapLogger(cfg Config) (Logger, error) {
	// 1. تنظیم سطح لاگ (Default: Info)
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	// 2. تنظیمات ساختار خروجی
	var encoder zapcore.Encoder
	if cfg.Encoding == "console" {
		encCfg := zap.NewDevelopmentEncoderConfig()
		encCfg.EncodeTime = zapcore.RFC3339NanoTimeEncoder
		encoder = zapcore.NewConsoleEncoder(encCfg)
	} else {
		encCfg := zap.NewProductionEncoderConfig()
		encCfg.TimeKey = "timestamp"
		encCfg.EncodeTime = zapcore.RFC3339NanoTimeEncoder
		encoder = zapcore.NewJSONEncoder(encCfg)
	}

	// 3. تعیین خروجی (فایل هوشمند یا کنسول)
	var writeSyncer zapcore.WriteSyncer
	if cfg.Output == "file" {
		rotator, err := rotatelogs.New(
			// نام‌گذاری روزانه: یک فایل به ازای هر روز
			cfg.FilePath+"-%Y-%m-%d.log",

			// ایجاد یک فایل میانبر همیشه آپدیت به نام اصلی
			rotatelogs.WithLinkName(cfg.FilePath+".log"),

			// نگهداری فایل‌ها تا حداکثر 30 روز
			rotatelogs.WithMaxAge(30*24*time.Hour),

			// چرخش روزانه در ابتدای هر شبانه‌روز
			rotatelogs.WithRotationTime(24*time.Hour),

			// ایجاد فایل جدید به محض رسیدن به حجم 50 مگابایت
			rotatelogs.WithRotationSize(50*1024*1024),
		)
		if err != nil {
			return nil, err
		}
		writeSyncer = zapcore.AddSync(rotator)
	} else {
		writeSyncer = zapcore.AddSync(os.Stdout)
	}

	// 4. ساخت هسته لاگر
	core := zapcore.NewCore(encoder, writeSyncer, level)

	// 5. ساخت نمونه نهایی Zap
	// CallerSkip(1): چون ما متدهای Zap را داخل متدهای خودمان (مثلا Info) فراخوانی می‌کنیم،
	// باید یک لایه به عقب برگردد تا نام فایل و خطِ هندلر اصلی را چاپ کند، نه نام این فایل را.
	zLog := zap.New(core,
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.ErrorLevel), // استک‌تریس فقط برای ارورها
	)

	logger := &zapLogger{logger: zLog}
	loggerForErrorGeneral = logger

	return logger, nil
}

// extractContextFields در صورت وجود TraceID در کانتکست، آن را به عنوان یک Field استخراج می‌کند
func (z *zapLogger) extractContextFields(ctx context.Context, fields []zap.Field) []zap.Field {
	if ctx == nil {
		return fields
	}

	if traceID, ok := ctx.Value(TraceIDKey).(string); ok && traceID != "" {
		// TraceID را به ابتدای فیلدها اضافه می‌کند تا در JSON اول چاپ شود
		return append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
	}

	return fields
}

// --- پیاده‌سازی متدهای اینترفیس ---

func (z *zapLogger) Debug(ctx context.Context, msg string, fields ...zap.Field) {
	z.logger.Debug(msg, z.extractContextFields(ctx, fields)...)
}

func (z *zapLogger) Info(ctx context.Context, msg string, fields ...zap.Field) {
	z.logger.Info(msg, z.extractContextFields(ctx, fields)...)
}

func (z *zapLogger) Warn(ctx context.Context, msg string, fields ...zap.Field) {
	z.logger.Warn(msg, z.extractContextFields(ctx, fields)...)
}

func (z *zapLogger) Error(ctx context.Context, msg string, fields ...zap.Field) {
	z.logger.Error(msg, z.extractContextFields(ctx, fields)...)
}

func (z *zapLogger) Fatal(ctx context.Context, msg string, fields ...zap.Field) {
	z.logger.Fatal(msg, z.extractContextFields(ctx, fields)...)
}

func (z *zapLogger) Sync() error {
	return z.logger.Sync()
}

func ErrorGeneral(ctx context.Context, err error, fields ...zap.Field) {
	loggerForErrorGeneral.Error(ctx, err.Error(), fields...)
}
