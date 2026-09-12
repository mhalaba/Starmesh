import 'package:flutter_test/flutter_test.dart';
import 'package:starmesh/main.dart';

void main() {
  testWidgets('Starmesh shell renders chat and network tabs', (tester) async {
    await tester.pumpWidget(const StarmeshApp(autoStart: false));
    expect(find.text('Chat'), findsWidgets);
    expect(find.text('Network'), findsOneWidget);
    expect(find.textContaining('No hub'), findsOneWidget);
  });
}
