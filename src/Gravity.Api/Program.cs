using System.Text.Json;
using Amazon.DynamoDBv2;
using Amazon.Lambda.AspNetCoreServer.Hosting;
using Gravity.Api.Common;
using Gravity.Api.Data;
using Microsoft.AspNetCore.Authentication.JwtBearer;
using Microsoft.IdentityModel.Tokens;

DotNetEnv.Env.TraversePath().Load();

var builder = WebApplication.CreateBuilder(args);

// --- Configuration guards (port of the checks at the top of src/app.ts) ---

var isProduction = Environment.GetEnvironmentVariable("NODE_ENV") == "production";
var clientOrigin = Environment.GetEnvironmentVariable("CLIENT_ORIGIN");

if (isProduction && string.IsNullOrWhiteSpace(clientOrigin))
	throw new InvalidOperationException("CLIENT_ORIGIN must be set in production");

var jwtSecret = Environment.GetEnvironmentVariable("JWT_SECRET")
	?? throw new InvalidOperationException("JWT_SECRET is missing");

// --- Services ---

builder.Services.AddSingleton<IAmazonDynamoDB>(_ =>
{
	// A local endpoint means DynamoDB Local, which accepts any credentials.
	var endpoint = Environment.GetEnvironmentVariable("DYNAMODB_ENDPOINT");

	if (string.IsNullOrWhiteSpace(endpoint)) return new AmazonDynamoDBClient();

	return new AmazonDynamoDBClient("local", "local", new AmazonDynamoDBConfig { ServiceURL = endpoint });
});

builder.Services.AddSingleton<IdGenerator>();
builder.Services.AddSingleton(new JwtService(jwtSecret));

builder.Services
	.AddAuthentication(JwtBearerDefaults.AuthenticationScheme)
	.AddJwtBearer(options =>
	{
		// Keep the raw "userId" claim instead of remapping it to a SOAP URI.
		options.MapInboundClaims = false;

		options.TokenValidationParameters = new TokenValidationParameters
		{
			ValidateIssuerSigningKey = true,
			IssuerSigningKey = new JwtService(jwtSecret).SigningKey,
			ValidateIssuer = false,
			ValidateAudience = false,
			ValidateLifetime = true,
			ClockSkew = TimeSpan.Zero,
		};

		// Express threw AppError("Token is missing", 401, "TOKEN") for both a
		// missing and an invalid token; the default 401 has an empty body, so
		// the envelope is written explicitly here.
		options.Events = new JwtBearerEvents
		{
			OnChallenge = async context =>
			{
				context.HandleResponse();

				if (context.Response.HasStarted) return;

				context.Response.StatusCode = 401;
				context.Response.ContentType = "application/json; charset=utf-8";

				await context.Response.WriteAsync(
					JsonSerializer.Serialize(new { error = "TOKEN" }, new JsonSerializerOptions(JsonSerializerDefaults.Web)));
			},
		};
	});

builder.Services.AddAuthorization();

builder.Services.AddCors(options => options.AddDefaultPolicy(policy =>
{
	// Production pins the single configured origin; development reflects any,
	// matching `allowedOrigins = [true]` in src/app.ts.
	if (isProduction) policy.WithOrigins(clientOrigin!);
	else policy.SetIsOriginAllowed(_ => true);

	policy.WithMethods("GET", "POST", "PATCH", "DELETE").AllowAnyHeader();
}));

builder.Services.AddAWSLambdaHosting(LambdaEventSource.HttpApi);

var app = builder.Build();

app.UseMiddleware<ExceptionMiddleware>();
app.UseCors();
app.UseAuthentication();
app.UseAuthorization();

app.MapGet("/", () => Results.Ok(new { status = "ok" }));

app.Run();
