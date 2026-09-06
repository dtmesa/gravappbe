using System.Text.Json.Serialization;
using Amazon.Lambda.APIGatewayEvents;

namespace Gravity.Api.Common;

/// <summary>
/// Serializer metadata for the Lambda event envelope itself -- the API Gateway
/// payload wrapping each request -- which is separate from the application's own
/// wire types in <see cref="GravityJsonContext"/>. AddAWSLambdaHosting otherwise
/// reflects over these, which does not survive ahead-of-time compilation.
/// </summary>
[JsonSerializable(typeof(APIGatewayHttpApiV2ProxyRequest))]
[JsonSerializable(typeof(APIGatewayHttpApiV2ProxyResponse))]
public partial class LambdaEventJsonContext : JsonSerializerContext;
